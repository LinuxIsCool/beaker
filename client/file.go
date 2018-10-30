package client

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
)

// FileHandle provides operations on a file within a dataset.
type FileHandle struct {
	dataset *DatasetHandle
	file    string
}

// FileRef creates an actor for an existing file within a dataset.
// This call doesn't perform any network operations.
func (h *DatasetHandle) FileRef(filePath string) *FileHandle {
	return &FileHandle{h, filePath}
}

// Download gets a file from a datastore.
func (h *FileHandle) Download(ctx context.Context) (io.ReadCloser, error) {
	path := path.Join("/api/v3/datasets", h.dataset.id, "files", h.file)
	req, err := h.dataset.client.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := errorFromResponse(resp); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// DownloadRange reads a range of bytes from a file.
// If length is negative, the file is read until the end.
func (h *FileHandle) DownloadRange(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
	path := path.Join("/api/v3/datasets", h.dataset.id, "files", h.file)
	req, err := h.dataset.client.newRequest(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	if length < 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if err := errorFromResponse(resp); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// DownloadTo downloads a file and writes it to disk.
func (h *FileHandle) DownloadTo(ctx context.Context, filePath string) error {
	r, err := h.Download(ctx)
	if err != nil {
		return err
	}
	defer safeClose(r)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return errors.WithStack(err)
	}

	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return errors.WithStack(err)
	}
	defer safeClose(f)

	_, err = io.Copy(f, r)
	return errors.WithStack(err)
}

func (h *FileHandle) Upload(ctx context.Context, source string) error {
	file, err := os.Open(source)
	if err != nil {
		return errors.WithStack(err)
	}
	defer file.Close()

	hasher := sha256.New()
	length, err := io.Copy(hasher, file)
	if err != nil {
		return errors.Wrap(err, "failed to hash contents")
	}

	digest := hasher.Sum(nil)

	if _, err := file.Seek(0, 0); err != nil {
		return errors.WithStack(err)
	}

	path := path.Join("/api/v3/datasets", h.dataset.id, "files", h.file)
	req, err := h.dataset.client.newRequest(ctx, http.MethodPut, path, nil, file)
	if err != nil {
		return err
	}
	req.ContentLength = length
	req.Header.Set("Digest", "SHA256 "+base64.StdEncoding.EncodeToString(digest))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return errors.WithStack(err)
	}
	return errorFromResponse(resp)
}

// Delete removes a file from an uncommitted datastore.
func (h *FileHandle) Delete(ctx context.Context) error {
	path := path.Join("/api/v3/datasets", h.dataset.id, "files", h.file)
	resp, err := h.dataset.client.sendRequest(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return err
	}
	defer safeClose(resp.Body)
	return errorFromResponse(resp)
}
