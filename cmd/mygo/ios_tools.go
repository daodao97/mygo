package main

import "fmt"

const iosToolUsage = `iOS tools:
  mygo ios check   verify an app, archive or IPA and its source symbols
  mygo ios doctor  check Xcode, SDKs, signing and local packaging tools

Use "mygo build -platform ios/arm64" to build a native app and Xcode project.
Add -ios-archive and -ios-export-method to create an archive and IPA.
Run "mygo ios <command> -h" for flags.
`

func runIOS(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Print(iosToolUsage)
		return nil
	}
	switch args[0] {
	case "check":
		return runIOSCheck(args[1:])
	case "doctor":
		return runIOSDoctor(args[1:])
	default:
		return fmt.Errorf("unknown iOS command %q; run mygo ios -h", args[0])
	}
}
