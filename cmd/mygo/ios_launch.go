package main

import (
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

func iosLaunchTitle(c *Config) string {
	if c.IOS.LaunchScreen.Title != "" {
		return c.IOS.LaunchScreen.Title
	}
	return c.Name
}

func iosLaunchColor(key, value, system string) string {
	if value == "" {
		return fmt.Sprintf(`<color key="%s" systemColor="%s"/>`, key, system)
	}
	v, _ := strconv.ParseUint(value[1:], 16, 32)
	return fmt.Sprintf(`<color key="%s" red="%f" green="%f" blue="%f" alpha="1" colorSpace="custom" customColorSpace="sRGB"/>`, key, float64(v>>16)/255, float64((v>>8)&255)/255, float64(v&255)/255)
}

func iosLaunchAssets(c *Config, host string) error {
	l := c.IOS.LaunchScreen
	for _, color := range []string{l.BackgroundColor, l.ForegroundColor} {
		if color != "" && !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(color) {
			return fmt.Errorf("ios.launchScreen colors must be #RRGGBB")
		}
	}
	if l.FadeDurationMs < 0 || l.FadeDurationMs > 5000 {
		return fmt.Errorf("ios.launchScreen.fadeDurationMs must be between 0 and 5000")
	}
	logo := filepath.Join(host, "LaunchLogo.png")
	if l.Image != "" {
		f, err := os.Open(c.path(l.Image))
		if err != nil {
			return err
		}
		_, err = png.DecodeConfig(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("ios.launchScreen.image must be a PNG: %w", err)
		}
		if err := copyResource(c.path(l.Image), logo); err != nil {
			return err
		}
	} else {
		// Keep the exported project uniform, with no visible image by default.
		f, err := os.Create(logo)
		if err != nil {
			return err
		}
		err = png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return os.WriteFile(filepath.Join(host, "LaunchScreen.storyboard"), []byte(iosLaunchStoryboard(c)), 0o644)
}

func iosLaunchStoryboard(c *Config) string {
	l := c.IOS.LaunchScreen
	imageView, imageConstraints, titleOffset := "", "", "0"
	if l.Image != "" {
		titleOffset = "70"
		imageView = `<imageView contentMode="scaleAspectFit" image="LaunchLogo.png" translatesAutoresizingMaskIntoConstraints="NO" id="launch-logo"><rect key="frame" x="136.5" y="336" width="120" height="120"/><constraints><constraint firstAttribute="width" constant="120" id="logo-width"/><constraint firstAttribute="height" constant="120" id="logo-height"/></constraints></imageView>`
		imageConstraints = `<constraint firstItem="launch-logo" firstAttribute="centerX" secondItem="launch-view" secondAttribute="centerX" id="logo-x"/><constraint firstItem="launch-logo" firstAttribute="centerY" secondItem="launch-view" secondAttribute="centerY" constant="-30" id="logo-y"/>`
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<document type="com.apple.InterfaceBuilder3.CocoaTouch.Storyboard.XIB" version="3.0" toolsVersion="23501" targetRuntime="iOS.CocoaTouch" propertyAccessControl="none" useAutolayout="YES" launchScreen="YES" useSafeAreas="YES" colorMatched="YES" initialViewController="launch-controller">
<device id="retina6_12" orientation="portrait" appearance="light"/>
<dependencies><deployment identifier="iOS"/><plugIn identifier="com.apple.InterfaceBuilder.IBCocoaTouchPlugin" version="23501"/><capability name="Safe area layout guides" minToolsVersion="9.0"/><capability name="System colors in document resources" minToolsVersion="11.0"/></dependencies>
<scenes><scene sceneID="launch-scene"><objects><viewController id="launch-controller" sceneMemberID="viewController"><view key="view" contentMode="scaleToFill" id="launch-view"><rect key="frame" x="0" y="0" width="393" height="852"/><autoresizingMask key="autoresizingMask" widthSizable="YES" heightSizable="YES"/><subviews>%s
<label opaque="NO" userInteractionEnabled="NO" contentMode="left" text="%s" textAlignment="center" numberOfLines="0" translatesAutoresizingMaskIntoConstraints="NO" id="launch-title"><rect key="frame" x="20" y="406" width="353" height="40"/><fontDescription key="fontDescription" type="boldSystem" pointSize="32"/>%s</label>
</subviews><viewLayoutGuide key="safeArea" id="launch-safe"/>%s
<constraints>%s<constraint firstItem="launch-title" firstAttribute="centerX" secondItem="launch-view" secondAttribute="centerX" id="title-x"/><constraint firstItem="launch-title" firstAttribute="centerY" secondItem="launch-view" secondAttribute="centerY" constant="%s" id="title-y"/><constraint firstItem="launch-title" firstAttribute="leading" secondItem="launch-safe" secondAttribute="leading" constant="20" id="title-leading"/><constraint firstItem="launch-safe" firstAttribute="trailing" secondItem="launch-title" secondAttribute="trailing" constant="20" id="title-trailing"/></constraints>
</view></viewController><placeholder placeholderIdentifier="IBFirstResponder" id="launch-responder" sceneMemberID="firstResponder"/></objects></scene></scenes>
<resources><image name="LaunchLogo.png" width="120" height="120"/><systemColor name="systemBackgroundColor"><color white="1" alpha="1" colorSpace="custom" customColorSpace="genericGamma22GrayColorSpace"/></systemColor><systemColor name="labelColor"><color white="0" alpha="1" colorSpace="custom" customColorSpace="genericGamma22GrayColorSpace"/></systemColor></resources>
</document>
`, imageView, html.EscapeString(iosLaunchTitle(c)), iosLaunchColor("textColor", l.ForegroundColor, "labelColor"), iosLaunchColor("backgroundColor", l.BackgroundColor, "systemBackgroundColor"), imageConstraints, titleOffset)
}
