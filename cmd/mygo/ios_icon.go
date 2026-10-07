package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// The standard icon setting also feeds Xcode's app icon asset catalog.
// Xcode derives device sizes from one opaque 1024px source image.
func iosIconAssets(c *Config, host string) error {
	if c.Icon == "" {
		return nil
	}
	src, err := os.ReadFile(c.path(c.Icon))
	if err != nil {
		return fmt.Errorf("icon %s: %w", c.Icon, err)
	}
	if err := validateIcon(src); err != nil {
		return fmt.Errorf("icon %s: %w", c.Icon, err)
	}
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return fmt.Errorf("icon %s: %w", c.Icon, err)
	}
	// Desktop icons can be transparent. Flatten the iOS default appearance
	// over white; the OS applies its own corner mask after installation.
	opaque := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	draw.Draw(opaque, opaque.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(opaque, opaque.Bounds(), resize(img, 1024), image.Point{}, draw.Over)
	var data bytes.Buffer
	if err := png.Encode(&data, opaque); err != nil {
		return err
	}
	root := filepath.Join(host, "Assets.xcassets")
	set := filepath.Join(root, "AppIcon.appiconset")
	if err := os.MkdirAll(set, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(set, "AppIcon.png"), data.Bytes(), 0o644); err != nil {
		return err
	}
	info := map[string]any{"author": "xcode", "version": 1}
	for path, contents := range map[string]any{
		filepath.Join(root, "Contents.json"): map[string]any{"info": info},
		filepath.Join(set, "Contents.json"): map[string]any{
			"info":   info,
			"images": []any{map[string]any{"filename": "AppIcon.png", "idiom": "universal", "platform": "ios", "size": "1024x1024"}},
		},
	} {
		b, err := json.MarshalIndent(contents, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}
