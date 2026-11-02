//go:build (ios && cgo) || mygo_ios_clipboard

package ios

import (
	"bytes"
	"errors"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/transfer"
	"testing"
)

func TestPrepareClipboardRichMessage(t *testing.T) {
	data := transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.Text, []byte("你好👋\nMessage")),
		transfer.Bytes(transfer.HTML, []byte("<p>你好👋</p><p>Message</p>")),
		transfer.Bytes(transfer.Markdown, []byte("**你好👋**\n\nMessage")),
	))
	got, err := prepareClipboard(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.text == nil || *got.text != "你好👋\nMessage" || got.html == nil || *got.html != "<p>你好👋</p><p>Message</p>" || got.markdown == nil || *got.markdown != "**你好👋**\n\nMessage" {
		t.Fatalf("rich copy lost representations: %+v", got)
	}
}

func TestPrepareClipboardFailureIsAtomic(t *testing.T) {
	failure := errors.New("provider failure")
	for _, representation := range []transfer.Representation{
		transfer.Lazy(transfer.HTML, func() ([]byte, error) { return nil, failure }),
		transfer.Bytes(transfer.Format("application/x-unsupported"), []byte("value")),
		transfer.Bytes(transfer.PNG, nil),
	} {
		data := transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, []byte("partial")), representation))
		got, err := prepareClipboard(data)
		if err == nil || got.text != nil || got.html != nil || got.markdown != nil || got.png != nil {
			t.Fatalf("partial clipboard write: %+v %v", got, err)
		}
	}
	_, err := prepareClipboard(transfer.New(transfer.NewItem(transfer.Lazy(transfer.HTML, func() ([]byte, error) { return nil, failure }))))
	if !errors.Is(err, failure) {
		t.Fatalf("provider error lost: %v", err)
	}
	_, err = prepareClipboard(transfer.New(transfer.NewItem(transfer.Bytes("application/x-unsupported", nil))))
	if !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("unsupported error lost: %v", err)
	}
}

func TestPrepareClipboardEmptyTextAndImage(t *testing.T) {
	got, err := prepareClipboard(transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, nil), transfer.Bytes(transfer.PNG, []byte("png")))))
	if err != nil || got.text == nil || *got.text != "" || !bytes.Equal(got.png, []byte("png")) {
		t.Fatalf("empty text/image not preserved: %+v %v", got, err)
	}
	got, err = prepareClipboard(transfer.Data{})
	if err != nil || got.text != nil || got.html != nil || got.markdown != nil || len(got.png) != 0 {
		t.Fatalf("clear was not empty: %+v %v", got, err)
	}
}
