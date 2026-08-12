package backupstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

func manifestPath(root, backupID string) string {
	return filepath.Join(root, "manifests", backupID+".json")
}

func objectPath(root, digest string) string {
	if len(digest) < 2 {
		return filepath.Join(root, "objects", "sha256", "invalid", digest)
	}
	return filepath.Join(root, "objects", "sha256", digest[:2], digest)
}

func newBackupID() (string, error) {
	identifier := make([]byte, 32)
	if _, err := rand.Read(identifier); err != nil {
		return "", operation.Wrap(operation.KindFilesystem, "create_backup_manifest", "", errors.New("backup identifier could not be generated"))
	}
	return hex.EncodeToString(identifier), nil
}

func validateCaptureRequest(request CaptureRequest) (CaptureRequest, error) {
	clean := filepath.Clean(request.TargetPath)
	if request.TargetPath == "" || strings.Contains(request.TargetPath, "\x00") || !filepath.IsAbs(clean) {
		return CaptureRequest{}, operation.New(operation.KindInvalidPath, "backup target path must be absolute")
	}
	request.TargetPath = clean
	switch request.SourceOperation {
	case SourceOperationEdit, SourceOperationPatchPackage, SourceOperationRestore, SourceOperationManageBOM, SourceOperationConvertEncoding:
	default:
		return CaptureRequest{}, operation.New(operation.KindInvalidInput, "backup source operation is invalid")
	}
	if len(request.Label) > maxLabelBytes || !utf8.ValidString(request.Label) || strings.Contains(request.Label, "\x00") {
		return CaptureRequest{}, operation.New(operation.KindInvalidInput, "backup label is invalid")
	}
	return request, nil
}

func finalizeManifestChecksum(manifest Manifest) (Manifest, error) {
	manifest.ManifestChecksum = ""
	payload, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, operation.Wrap(operation.KindFilesystem, "checksum_backup_manifest", "", errors.New("backup manifest could not be encoded"))
	}
	digest := sha256.Sum256(payload)
	manifest.ManifestChecksum = hex.EncodeToString(digest[:])
	return manifest, nil
}

func validateManifest(manifest Manifest, descriptor Descriptor) error {
	if manifest.FormatVersion != ManifestVersion || manifest.StoreFormatVersion != FormatVersion ||
		manifest.StoreID != descriptor.StoreID || manifest.ObjectAlgorithm != ObjectAlgorithm {
		return operation.New(operation.KindInvalidInput, "backup manifest uses an unsupported format")
	}
	if !validHexIdentifier(manifest.BackupID) || !validHexIdentifier(manifest.ObjectDigest) || !validHexIdentifier(manifest.ManifestChecksum) {
		return operation.New(operation.KindInvalidInput, "backup manifest identifier or digest is invalid")
	}
	if manifest.TargetPath == "" || strings.Contains(manifest.TargetPath, "\x00") ||
		!filepath.IsAbs(manifest.TargetPath) || filepath.Clean(manifest.TargetPath) != manifest.TargetPath {
		return operation.New(operation.KindInvalidInput, "backup manifest target path is invalid")
	}
	switch manifest.SourceOperation {
	case SourceOperationEdit, SourceOperationPatchPackage, SourceOperationRestore, SourceOperationManageBOM, SourceOperationConvertEncoding:
	default:
		return operation.New(operation.KindInvalidInput, "backup manifest source operation is invalid")
	}
	if manifest.ObjectBytes < 0 || len(manifest.ContentFingerprint) != 64 || !validHexIdentifier(manifest.ContentFingerprint) {
		return operation.New(operation.KindInvalidInput, "backup manifest object evidence is invalid")
	}
	expectedFingerprint, err := filesystem.FingerprintRegularFileContentDigest(manifest.ObjectBytes, manifest.ObjectDigest)
	if err != nil || expectedFingerprint != manifest.ContentFingerprint {
		return operation.New(operation.KindInvalidInput, "backup manifest content fingerprint is inconsistent")
	}
	if manifest.OriginalMode > 0o777 {
		return operation.New(operation.KindInvalidInput, "backup manifest original mode is invalid")
	}
	if len(manifest.Label) > maxLabelBytes || !utf8.ValidString(manifest.Label) || strings.Contains(manifest.Label, "\x00") {
		return operation.New(operation.KindInvalidInput, "backup manifest label is invalid")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, manifest.CreatedAt)
	if err != nil || createdAt.Location() != time.UTC || !strings.HasSuffix(manifest.CreatedAt, "Z") {
		return operation.New(operation.KindInvalidInput, "backup manifest creation timestamp is invalid")
	}
	modTime, err := time.Parse(time.RFC3339Nano, manifest.OriginalModTime)
	if err != nil || modTime.Location() != time.UTC || !strings.HasSuffix(manifest.OriginalModTime, "Z") {
		return operation.New(operation.KindInvalidInput, "backup manifest modification timestamp is invalid")
	}
	expected, err := finalizeManifestChecksum(manifest)
	if err != nil {
		return err
	}
	if expected.ManifestChecksum != manifest.ManifestChecksum {
		return operation.New(operation.KindFilesystem, "backup manifest checksum is invalid")
	}
	return nil
}

func validHexIdentifier(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func encodeManifest(manifest Manifest) ([]byte, error) {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, operation.Wrap(operation.KindFilesystem, "encode_backup_manifest", "", errors.New("backup manifest could not be encoded"))
	}
	data = append(data, '\n')
	if len(data) > maxManifestBytes {
		return nil, operation.New(operation.KindLimit, "backup manifest exceeds the maximum encoded size")
	}
	return data, nil
}

func readManifest(path string, lstatInfo fs.FileInfo, descriptor Descriptor) (Manifest, error) {
	if lstatInfo == nil || isLinkOrReparse(lstatInfo) || !lstatInfo.Mode().IsRegular() || lstatInfo.Size() > maxManifestBytes {
		return Manifest{}, operation.New(operation.KindFilesystem, "backup manifest metadata is invalid")
	}
	if err := validateSingleLink(path, lstatInfo); err != nil {
		return Manifest{}, operation.New(operation.KindFilesystem, "backup manifest hard-link state is invalid")
	}
	if err := validatePathPermissions(path, false); err != nil {
		return Manifest{}, sanitizedFilesystemError("backup manifest permissions are not owner-only", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, sanitizedFilesystemError("backup manifest cannot be opened", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Manifest{}, sanitizedFilesystemError("backup manifest cannot be inspected", err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(lstatInfo, info) || info.Size() > maxManifestBytes {
		return Manifest{}, operation.New(operation.KindFilesystem, "backup manifest identity or size is invalid")
	}
	return decodeManifest(io.LimitReader(file, maxManifestBytes+1), descriptor)
}

func decodeManifest(reader io.Reader, descriptor Descriptor) (Manifest, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, operation.New(operation.KindInvalidInput, "backup manifest is malformed")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Manifest{}, operation.New(operation.KindInvalidInput, "backup manifest contains trailing data")
	}
	if err := validateManifest(manifest, descriptor); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
