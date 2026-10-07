# Image fixtures

Original geometric samples generated with Go standard image encoders.
The same red, blue, yellow and green quadrants make fit, color sampling and
orientation errors visible without a network connection.

- PNG, JPEG and lossless WebP contain the upright 240 × 120 pattern.
- `oriented.jpg` stores 120 × 240 pixels with EXIF orientation 6. Correct
  decoding produces the same upright 240 × 120 pattern.
- GIF contains two different frames. MyGo currently displays only the first.
- `transparent.png` has a blue circle with varying alpha and transparent
  corners, displayed over a yellow background to check blending.
- `launch-logo.png` follows the CLI template's blue-violet gradient and white
  ring, with a transparent rounded inset. `../resources/icon.png` uses the
  same mark on a full opaque square for the iOS home-screen icon.

From `examples/ios-native`, run `go run ./assets/generate.go` to regenerate.
The generator needs ffmpeg for WebP encoding; the app does not need it.
The colored SVG is embedded in `rendering.go` and exercises a gradient,
rounded clip, opacity and a mask.
