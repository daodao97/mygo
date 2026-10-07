package main

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// readIOSPlist accepts XML and binary Apple plists. Binary conversion is an
// Xcode-host operation; XML parsing remains testable without macOS tooling.
func readIOSPlist(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 4<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 4<<20 {
		return nil, fmt.Errorf("property list exceeds 4 MB: %s", path)
	}
	if bytes.HasPrefix(data, []byte("bplist00")) {
		data, err = exec.Command("plutil", "-convert", "xml1", "-o", "-", path).Output()
		if err != nil {
			return nil, fmt.Errorf("converting binary property list: %w", err)
		}
		if len(data) > 4<<20 {
			return nil, fmt.Errorf("converted property list exceeds 4 MB: %s", path)
		}
	}
	d, err := parseIOSPlist(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

func parseIOSPlist(data []byte) (map[string]any, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	next := func() (xml.Token, error) { return plistToken(d) }
	t, err := next()
	if err != nil {
		return nil, err
	}
	root, ok := t.(xml.StartElement)
	if !ok || root.Name.Local != "plist" {
		return nil, fmt.Errorf("expected plist root")
	}
	t, err = next()
	if err != nil {
		return nil, err
	}
	start, ok := t.(xml.StartElement)
	if !ok {
		return nil, fmt.Errorf("missing property list value")
	}
	v, err := plistValue(d, start, 0)
	if err != nil {
		return nil, err
	}
	result, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("property list root must be a dictionary")
	}
	t, err = next()
	end, ok := t.(xml.EndElement)
	if err != nil || !ok || end.Name != root.Name {
		return nil, fmt.Errorf("invalid plist closing element")
	}
	if _, err := next(); err != io.EOF {
		return nil, fmt.Errorf("unexpected content after property list")
	}
	return result, nil
}

func plistToken(d *xml.Decoder) (xml.Token, error) {
	for {
		t, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch x := t.(type) {
		case xml.Comment, xml.ProcInst, xml.Directive:
			continue
		case xml.CharData:
			if strings.TrimSpace(string(x)) == "" {
				continue
			}
			return nil, fmt.Errorf("unexpected text in property list")
		}
		return t, nil
	}
}

func plistValue(d *xml.Decoder, start xml.StartElement, depth int) (any, error) {
	if depth > 32 {
		return nil, fmt.Errorf("property list is nested too deeply")
	}
	switch start.Name.Local {
	case "dict", "array":
		result, items := map[string]any{}, []any{}
		for {
			t, err := plistToken(d)
			if err != nil {
				return nil, err
			}
			if end, ok := t.(xml.EndElement); ok {
				if end.Name != start.Name {
					return nil, fmt.Errorf("mismatched property list element")
				}
				if start.Name.Local == "dict" {
					return result, nil
				}
				return items, nil
			}
			e, ok := t.(xml.StartElement)
			if !ok {
				return nil, fmt.Errorf("invalid property list element")
			}
			key := ""
			if start.Name.Local == "dict" {
				if e.Name.Local != "key" {
					return nil, fmt.Errorf("expected dictionary key")
				}
				if err := d.DecodeElement(&key, &e); err != nil {
					return nil, err
				}
				if _, exists := result[key]; exists {
					return nil, fmt.Errorf("duplicate property list key %q", key)
				}
				t, err = plistToken(d)
				if err != nil {
					return nil, err
				}
				e, ok = t.(xml.StartElement)
				if !ok {
					return nil, fmt.Errorf("missing value for %q", key)
				}
			}
			v, err := plistValue(d, e, depth+1)
			if err != nil {
				return nil, err
			}
			if start.Name.Local == "dict" {
				result[key] = v
			} else {
				items = append(items, v)
			}
		}
	case "string", "date", "data", "integer", "real", "true", "false":
		var value string
		if err := d.DecodeElement(&value, &start); err != nil {
			return nil, err
		}
		switch start.Name.Local {
		case "integer":
			return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		case "real":
			return strconv.ParseFloat(strings.TrimSpace(value), 64)
		case "data":
			return base64.StdEncoding.DecodeString(strings.Join(strings.Fields(value), ""))
		case "true", "false":
			if strings.TrimSpace(value) != "" {
				return nil, fmt.Errorf("invalid plist boolean")
			}
			return start.Name.Local == "true", nil
		default:
			return value, nil
		}
	default:
		return nil, fmt.Errorf("unsupported property list element %q", start.Name.Local)
	}
}
