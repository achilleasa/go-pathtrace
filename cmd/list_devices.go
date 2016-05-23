package cmd

import (
	"bytes"
	"fmt"
	"os"

	"github.com/achilleasa/go-pathtrace/tracer/opencl/device"
	"github.com/codegangsta/cli"
)

// List available opencl devices.
func ListDevices(ctx *cli.Context) {
	var storage []byte
	buf := bytes.NewBuffer(storage)

	clPlatforms, err := device.GetPlatformInfo()
	if err != nil {
		logger.Printf("error: could not list devices: %s", err.Error())
		os.Exit(1)
	}

	buf.WriteString(fmt.Sprintf("\nSystem provides %d opencl platform(s):\n\n", len(clPlatforms)))
	for pIdx, platformInfo := range clPlatforms {
		buf.WriteString(fmt.Sprintf("[Platform %02d]\n  Name    %s\n  Version %s\n  Profile %s\n  Devices %d\n\n", pIdx, platformInfo.Name, platformInfo.Version, platformInfo.Profile, len(platformInfo.Devices)))
		for dIdx, dev := range platformInfo.Devices {
			buf.WriteString(fmt.Sprintf("  [Device %02d]\n    Name  %s\n    Type  %s\n    Speed %d GFlops\n\n", dIdx, dev.Name, dev.Type, dev.Speed))
		}
	}

	logger.Print(buf.String())
}
