package compiler

import (
	"fmt"

	"github.com/achilleasa/go-pathtrace/scene"
	"github.com/achilleasa/go-pathtrace/types"
)

const (
	minPrimitivesPerLeaf = 10
)

type sceneCompiler struct {
	parsedScene    *scene.ParsedScene
	optimizedScene *scene.Scene
}

// Compile a scene representation parsed by a scene reader into a GPU-friendly
// optimized scene format.
func Compile(parsedScene *scene.ParsedScene) (*scene.Scene, error) {
	compiler := &sceneCompiler{
		parsedScene:    parsedScene,
		optimizedScene: &scene.Scene{},
	}

	err := compiler.bakeTextures()
	if err != nil {
		return nil, err
	}

	err = compiler.partitionGeometry()
	if err != nil {
		return nil, err
	}

	/*
		err = compiler.createLayeredMaterialTrees()
		if err != nil {
			return nil, err
		}
	*/
	err = compiler.setupCamera()
	if err != nil {
		return nil, err
	}

	return compiler.optimizedScene, nil
}

// Allocate a contiguous memory block for all texture data and initialize the
// scene's texture metadata so that they point to the proper index inside the block.
func (sc *sceneCompiler) bakeTextures() error {
	// Find how much memory we need. To ensure proper memory alignment we pad
	// each texture's data len so its a multiple of a qword
	var totalDataLen uint32 = 0
	for _, tex := range sc.parsedScene.Textures {
		totalDataLen += align4(len(tex.Data))
	}

	sc.optimizedScene.TextureData = make([]byte, totalDataLen)
	sc.optimizedScene.TextureMetadata = make([]scene.TextureMetadata, len(sc.parsedScene.Textures))
	var offset uint32 = 0
	for index, tex := range sc.parsedScene.Textures {

		sc.optimizedScene.TextureMetadata[index].Format = tex.Format
		sc.optimizedScene.TextureMetadata[index].Width = tex.Width
		sc.optimizedScene.TextureMetadata[index].Height = tex.Height
		sc.optimizedScene.TextureMetadata[index].DataOffset = offset

		// Copy data
		copy(sc.optimizedScene.TextureData[offset:], tex.Data)
		offset += uint32(align4(len(tex.Data)))
	}

	return nil
}

// Generate a two-level BVH tree for the scene. The top level BVH tree partitions
// the mesh instances. An additional BVH tree is also generated for each
// defined scene mesh. Each mesh instance points to the root BVH node of a mesh.
func (sc *sceneCompiler) partitionGeometry() error {

	// Partition mesh instances so that each instance ends up in its own BVH leaf.
	volList := make([]BoundedVolume, len(sc.parsedScene.MeshInstances))
	for index, mi := range sc.parsedScene.MeshInstances {
		volList[index] = mi
	}
	sc.optimizedScene.BvhNodeList = BuildBVH(volList, 1, func(node *scene.BvhNode, workList []BoundedVolume) {
		pmi := workList[0].(*scene.ParsedMeshInstance)

		// Assign mesh instance index to node
		for index, mi := range sc.parsedScene.MeshInstances {
			if pmi == mi {
				node.SetMeshIndex(uint32(index))
				break
			}
		}
	})

	// Scan all meshes and calculate the size of material, vertex, normal
	// and uv lists; then pre-allocate them.
	totalVertices := 0
	for _, pm := range sc.parsedScene.Meshes {
		totalVertices += 3 * len(pm.Primitives)
	}

	sc.optimizedScene.VertexList = make([]types.Vec4, totalVertices)
	sc.optimizedScene.NormalList = make([]types.Vec4, totalVertices)
	sc.optimizedScene.UvList = make([]types.Vec2, totalVertices)
	sc.optimizedScene.MaterialIndex = make([]uint32, totalVertices/3)

	// Partition each mesh into its own BVH. Update all instances to point to this mesh BVH.
	var vertexOffset uint32 = 0
	var primOffset uint32 = 0
	meshBvhRoots := make([]uint32, len(sc.parsedScene.Meshes))
	for mIndex, pm := range sc.parsedScene.Meshes {
		volList := make([]BoundedVolume, len(pm.Primitives))
		for index, prim := range pm.Primitives {
			volList[index] = prim
		}

		bvhNodes := BuildBVH(volList, minPrimitivesPerLeaf, func(node *scene.BvhNode, workList []BoundedVolume) {
			node.SetPrimitives(primOffset, uint32(len(workList)))

			// Copy primitive data to flat arrays
			for _, workItem := range workList {
				prim := workItem.(*scene.ParsedPrimitive)

				// Convert Vec3 to Vec4 which is required for proper alignment inside opencl kernels
				sc.optimizedScene.VertexList[vertexOffset+0] = prim.Vertices[0].Vec4(0)
				sc.optimizedScene.VertexList[vertexOffset+1] = prim.Vertices[1].Vec4(0)
				sc.optimizedScene.VertexList[vertexOffset+2] = prim.Vertices[2].Vec4(0)

				sc.optimizedScene.NormalList[vertexOffset+0] = prim.Normals[0].Vec4(0)
				sc.optimizedScene.NormalList[vertexOffset+1] = prim.Normals[1].Vec4(0)
				sc.optimizedScene.NormalList[vertexOffset+2] = prim.Normals[2].Vec4(0)

				sc.optimizedScene.UvList[vertexOffset+0] = prim.UVs[0]
				sc.optimizedScene.UvList[vertexOffset+1] = prim.UVs[1]

				sc.optimizedScene.MaterialIndex[primOffset] = prim.MaterialIndex

				vertexOffset += 3
				primOffset++
			}
		})

		// Apply offset to bvh nodes and append them to the scene bvh list
		offset := int32(len(sc.optimizedScene.BvhNodeList))
		meshBvhRoots[mIndex] = uint32(offset)
		for index, _ := range bvhNodes {
			bvhNodes[index].OffsetChildNodes(offset)
		}
		sc.optimizedScene.BvhNodeList = append(sc.optimizedScene.BvhNodeList, bvhNodes...)
	}

	// Process each mesh instance
	sc.optimizedScene.MeshInstanceList = make([]scene.MeshInstance, len(sc.parsedScene.MeshInstances))
	for index, pmi := range sc.parsedScene.MeshInstances {
		mi := &sc.optimizedScene.MeshInstanceList[index]
		mi.MeshIndex = pmi.MeshIndex
		mi.BvhRoot = meshBvhRoots[pmi.MeshIndex]

		// We need to invert the transformation matrix when performing ray traversal
		mi.Transform = pmi.Transform.Inv()
	}

	return nil
}

// Convert material definitions into a node-based structure that models a
// layered material.
func (sc *sceneCompiler) createLayeredMaterialTrees() error {
	return fmt.Errorf("sceneCompiler: createLayeredMaterialTrees() not yet implemented")
}

// Initialize and position the camera for the scene.
func (sc *sceneCompiler) setupCamera() error {
	sc.optimizedScene.Camera = scene.NewCamera(sc.parsedScene.Camera.FOV)
	sc.optimizedScene.Camera.Position = sc.parsedScene.Camera.Eye
	sc.optimizedScene.Camera.LookAt = sc.parsedScene.Camera.Look
	sc.optimizedScene.Camera.Up = sc.parsedScene.Camera.Up

	return nil
}

// Adjust value so its divisible by 4.
func align4(value int) uint32 {
	for {
		if value%4 == 0 {
			return uint32(value)
		}
		value++
	}
}
