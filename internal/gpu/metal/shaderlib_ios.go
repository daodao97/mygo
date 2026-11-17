package metal

import "os"

// shaderLibrary is the library compiled for where the app runs: the
// simulator sets SIMULATOR_DEVICE_NAME in its apps' environment.
var shaderLibrary = func() []byte {
	if os.Getenv("SIMULATOR_DEVICE_NAME") != "" {
		return shaderLibrarySimulator
	}
	return shaderLibraryDevice
}()
