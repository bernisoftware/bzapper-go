package bzapper

import (
	"bytes"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// UploadFile is a file sent as multipart/form-data (logos, campaign media).
type UploadFile struct {
	// Filename is the name sent to the API (e.g. "logo.png"). Required.
	Filename string
	// ContentType is the MIME type (e.g. "image/png"). Empty = guessed from the
	// Filename extension, else application/octet-stream.
	ContentType string
	// Content is the file bytes.
	Content []byte
}

// FileFromPath reads a file from disk into an UploadFile (name and MIME type
// taken from the path).
func FileFromPath(path string) (UploadFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return UploadFile{}, err
	}
	return UploadFile{Filename: filepath.Base(path), Content: data}, nil
}

// multipartRequest encodes the file (field "file") plus extra fields once, so
// every retry resends the same bytes.
func multipartRequest(method, path string, file UploadFile, fields map[string]string) (apiRequest, error) {
	if strings.TrimSpace(file.Filename) == "" {
		return apiRequest{}, argErr("UploadFile.Filename must not be empty")
	}
	contentType := file.ContentType
	if contentType == "" {
		contentType = mime.TypeByExtension(strings.ToLower(filepath.Ext(file.Filename)))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	names := make([]string, 0, len(fields))
	for k := range fields {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if err := w.WriteField(k, fields[k]); err != nil {
			return apiRequest{}, argErr(fmt.Sprintf("multipart field %q: %v", k, err))
		}
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, file.Filename))
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		return apiRequest{}, argErr(fmt.Sprintf("multipart file: %v", err))
	}
	if _, err := part.Write(file.Content); err != nil {
		return apiRequest{}, argErr(fmt.Sprintf("multipart file: %v", err))
	}
	if err := w.Close(); err != nil {
		return apiRequest{}, argErr(fmt.Sprintf("multipart: %v", err))
	}
	raw := buf.Bytes()
	if raw == nil {
		raw = []byte{}
	}
	return apiRequest{method: method, path: path, raw: raw, contentType: w.FormDataContentType()}, nil
}
