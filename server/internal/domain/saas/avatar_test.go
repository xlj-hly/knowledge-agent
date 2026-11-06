package domain

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestAvatarImageRejectsInvalidBytesAndDimensions(t *testing.T) {
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	var wideBody bytes.Buffer
	if err := png.Encode(&wideBody, image.NewRGBA(image.Rect(0, 0, 4097, 1))); err != nil {
		t.Fatal(err)
	}
	for _, body := range [][]byte{nil, []byte("forged png"), pngBody.Bytes()[:len(pngBody.Bytes())-8], make([]byte, 2*1024*1024+1), wideBody.Bytes()} {
		if _, err := ValidateAvatarImage(body); err == nil {
			t.Fatal("invalid avatar image accepted")
		}
	}
	if format, err := ValidateAvatarImage(pngBody.Bytes()); err != nil || format != "png" {
		t.Fatalf("valid PNG rejected: %q %v", format, err)
	}
	var jpegBody bytes.Buffer
	if err := jpeg.Encode(&jpegBody, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil); err != nil {
		t.Fatal(err)
	}
	if format, err := ValidateAvatarImage(jpegBody.Bytes()); err != nil || format != "jpeg" {
		t.Fatalf("valid JPEG rejected: %q %v", format, err)
	}
}
