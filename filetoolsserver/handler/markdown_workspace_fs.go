package handler

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/security"
)

type markdownWorkspaceFS struct {
	ctx      context.Context
	handler  *Handler
	root     string
	encoding string
}

func newMarkdownWorkspaceFS(ctx context.Context, handler *Handler, root, encoding string) (*markdownWorkspaceFS, error) {
	if handler == nil {
		return nil, fmt.Errorf("markdown workspace handler is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	validated, err := handler.validatePath(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(validated)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("markdown workspace root is not a directory: %s", root)
	}
	return &markdownWorkspaceFS{ctx: ctx, handler: handler, root: validated, encoding: encoding}, nil
}

func (workspace *markdownWorkspaceFS) Open(name string) (fs.File, error) {
	if workspace == nil || workspace.handler == nil || !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if err := workspace.ctx.Err(); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	candidate := workspace.root
	if name != "." {
		candidate = filepath.Join(workspace.root, filepath.FromSlash(name))
	}
	validated, err := workspace.handler.validatePath(candidate)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if !security.IsPathWithinAllowedDirectories(validated, []string{workspace.root}) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}

	info, err := os.Stat(validated)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if info.IsDir() {
		file, openErr := os.Open(validated)
		if openErr != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: openErr}
		}
		return file, nil
	}
	if !info.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	extension := strings.ToLower(filepath.Ext(validated))
	if extension != ".md" && extension != ".markdown" {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	document, _, err := workspace.handler.readTextDocumentWithData(workspace.ctx, validated, workspace.encoding)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	if !utf8.ValidString(document.Text) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fmt.Errorf("decoded Markdown is not valid UTF-8")}
	}
	data := []byte(document.Text)
	displayName := path.Base(name)
	if name == "." {
		displayName = filepath.Base(workspace.root)
	}
	return &markdownWorkspaceDecodedFile{
		Reader: bytes.NewReader(data),
		info: markdownWorkspaceFileInfo{
			name:    displayName,
			size:    int64(len(data)),
			mode:    info.Mode(),
			modTime: info.ModTime(),
		},
	}, nil
}

type markdownWorkspaceDecodedFile struct {
	*bytes.Reader
	info markdownWorkspaceFileInfo
}

func (file *markdownWorkspaceDecodedFile) Close() error               { return nil }
func (file *markdownWorkspaceDecodedFile) Stat() (fs.FileInfo, error) { return file.info, nil }

type markdownWorkspaceFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (info markdownWorkspaceFileInfo) Name() string       { return info.name }
func (info markdownWorkspaceFileInfo) Size() int64        { return info.size }
func (info markdownWorkspaceFileInfo) Mode() fs.FileMode  { return info.mode }
func (info markdownWorkspaceFileInfo) ModTime() time.Time { return info.modTime }
func (info markdownWorkspaceFileInfo) IsDir() bool        { return false }
func (info markdownWorkspaceFileInfo) Sys() any           { return nil }
