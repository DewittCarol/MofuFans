package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"io"
	"mofufans/internal/siteconfig"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Assets keeps application URLs and authorization independent of the backend.
// Directories are local-only; OSS keys depend on namespace and filename.
type Assets struct {
	Namespace, Directory string
	bucket               *oss.Bucket
}

func NewAssets(namespace, directory string) *Assets {
	return &Assets{Namespace: namespace, Directory: directory}
}

// NewOSSAssets accepts a configured SDK bucket, useful for embedded deployments
// and contract tests against an isolated OSS-compatible HTTP endpoint.
func NewOSSAssets(namespace string, bucket *oss.Bucket) *Assets {
	return &Assets{Namespace: namespace, bucket: bucket}
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func validName(name string) bool { return safeName.MatchString(name) && name != "." && name != ".." }
func (a *Assets) key(name string) (string, error) {
	if !validName(name) || !validName(a.Namespace) {
		return "", errors.New("invalid asset name")
	}
	prefix := strings.Trim(siteconfig.Active().OSS.Prefix, "/")
	for _, segment := range strings.Split(prefix, "/") {
		if segment != "" && !validName(segment) {
			return "", errors.New("invalid OSS prefix")
		}
	}
	return path.Join(prefix, a.Namespace, name), nil
}
func (a *Assets) cloud() bool { return a.bucket != nil || siteconfig.Active().StorageType == "oss" }
func configuredBucket() (*oss.Bucket, error) {
	return Bucket(siteconfig.Active())
}
func Bucket(options siteconfig.Options) (*oss.Bucket, error) {
	endpoint := options.OSS.Endpoint
	if !strings.HasPrefix(endpoint, "https://") {
		return nil, errors.New("oss.endpoint must use HTTPS")
	}
	for key, value := range map[string]string{"oss.region": options.OSS.Region, "oss.bucket": options.OSS.Bucket, "oss.access_key": options.OSS.AccessKey, "oss.secret_key": options.OSS.SecretKey} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("%s must be configured", key)
		}
	}
	client, err := oss.New(endpoint, options.OSS.AccessKey, options.OSS.SecretKey, oss.AuthVersion(oss.AuthV4), oss.Region(options.OSS.Region), oss.Timeout(15, 120))
	if err != nil {
		return nil, errors.New("invalid OSS configuration")
	}
	return client.Bucket(options.OSS.Bucket)
}
func Validate() error {
	switch siteconfig.Active().StorageType {
	case "local":
		return nil
	case "oss":
		if _, err := NewAssets("uploads", "").key("check"); err != nil {
			return err
		}
		_, err := configuredBucket()
		return err
	default:
		return errors.New("storage_type must be local or oss")
	}
}
func (a *Assets) remote() (*oss.Bucket, error) {
	if a.bucket != nil {
		return a.bucket, nil
	}
	return configuredBucket()
}
func (a *Assets) Put(ctx context.Context, name string, reader io.Reader) error {
	key, err := a.key(name)
	if err != nil {
		return err
	}
	if a.cloud() {
		b, e := a.remote()
		if e != nil {
			return e
		}
		return b.PutObject(key, reader, oss.ObjectACL(oss.ACLPrivate), oss.WithContext(ctx))
	}
	if err = os.MkdirAll(a.Directory, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(a.Directory, ".asset-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, copyErr := io.Copy(f, reader)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(a.Directory, name))
}

// Keep staging data until the caller's database commit succeeds, for safe retry.
func (a *Assets) PutFile(ctx context.Context, name, source string) error {
	key, err := a.key(name)
	if err != nil {
		return err
	}
	if !a.cloud() {
		if filepath.Clean(source) == filepath.Clean(filepath.Join(a.Directory, name)) {
			return nil
		}
		f, e := os.Open(source)
		if e != nil {
			return e
		}
		defer f.Close()
		return a.Put(ctx, name, f)
	}
	b, err := a.remote()
	if err != nil {
		return err
	}
	stat, err := os.Stat(source)
	if err != nil {
		return err
	}
	if stat.Size() <= 16<<20 {
		return b.PutObjectFromFile(key, source, oss.ObjectACL(oss.ACLPrivate), oss.WithContext(ctx))
	}
	partSize := int64(16 << 20)
	if n := (stat.Size() + 9998) / 9999; n > partSize {
		partSize = n
	}
	return b.UploadFile(key, source, partSize, oss.Routines(2), oss.ObjectACL(oss.ACLPrivate), oss.WithContext(ctx))
}
func (a *Assets) ReleaseStaging(source string) {
	if a.cloud() {
		_ = os.Remove(source)
	}
}

// RollbackFile removes an uncommitted destination while keeping the staged
// upload available for a retry. Local uploads can already use the final path.
func (a *Assets) RollbackFile(ctx context.Context, name, source string) error {
	if !a.cloud() && filepath.Clean(source) == filepath.Clean(filepath.Join(a.Directory, name)) {
		return nil
	}
	return a.Delete(ctx, name)
}
func cloudError(err error) error {
	var e oss.ServiceError
	if errors.As(err, &e) && e.StatusCode == 404 {
		return os.ErrNotExist
	}
	return err
}

type assetInfo struct {
	name     string
	size     int64
	modified time.Time
}

func (i assetInfo) Name() string       { return i.name }
func (i assetInfo) Size() int64        { return i.size }
func (i assetInfo) Mode() os.FileMode  { return 0600 }
func (i assetInfo) ModTime() time.Time { return i.modified }
func (i assetInfo) IsDir() bool        { return false }
func (i assetInfo) Sys() any           { return nil }
func (a *Assets) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	key, err := a.key(name)
	if err != nil {
		return nil, err
	}
	if !a.cloud() {
		info, e := os.Lstat(filepath.Join(a.Directory, name))
		if e == nil && !info.Mode().IsRegular() {
			return nil, os.ErrNotExist
		}
		return info, e
	}
	b, err := a.remote()
	if err != nil {
		return nil, err
	}
	h, err := b.GetObjectDetailedMeta(key, oss.WithContext(ctx))
	if err != nil {
		return nil, cloudError(err)
	}
	size, err := strconv.ParseInt(h.Get("Content-Length"), 10, 64)
	if err != nil || size < 0 {
		return nil, errors.New("invalid OSS object size")
	}
	modified, _ := http.ParseTime(h.Get("Last-Modified"))
	return assetInfo{name, size, modified}, nil
}
func (a *Assets) Delete(ctx context.Context, name string) error {
	key, err := a.key(name)
	if err != nil {
		return err
	}
	if !a.cloud() {
		err = os.Remove(filepath.Join(a.Directory, name))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	b, err := a.remote()
	if err != nil {
		return err
	}
	return b.DeleteObject(key, oss.WithContext(ctx))
}
func (a *Assets) List(ctx context.Context) ([]os.FileInfo, error) {
	result := []os.FileInfo{}
	if !a.cloud() {
		rows, err := os.ReadDir(a.Directory)
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Type().IsRegular() {
				i, e := row.Info()
				if e != nil {
					return nil, e
				}
				result = append(result, i)
			}
		}
		return result, nil
	}
	key, err := a.key("placeholder")
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(key, "placeholder")
	b, err := a.remote()
	if err != nil {
		return nil, err
	}
	marker := ""
	for {
		r, e := b.ListObjects(oss.Prefix(prefix), oss.Marker(marker), oss.MaxKeys(1000), oss.WithContext(ctx))
		if e != nil {
			return nil, e
		}
		for _, obj := range r.Objects {
			name := strings.TrimPrefix(obj.Key, prefix)
			if validName(name) {
				result = append(result, assetInfo{name, obj.Size, obj.LastModified})
			}
		}
		if !r.IsTruncated {
			break
		}
		if r.NextMarker == marker {
			return nil, errors.New("OSS pagination stalled")
		}
		marker = r.NextMarker
	}
	return result, nil
}

type AssetFile interface {
	io.ReadSeekCloser
	Stat() (os.FileInfo, error)
}

// Materialize provides bounded, local originals for image variant generation.
// Callers use content-addressed names; cached originals never contain credentials.
func (a *Assets) Materialize(ctx context.Context, name string, limit int64) (string, error) {
	if !validName(name) {
		return "", os.ErrNotExist
	}
	if !a.cloud() {
		if _, err := a.Stat(ctx, name); err != nil {
			return "", err
		}
		return filepath.Join(a.Directory, name), nil
	}
	dir := filepath.Join(a.Directory, ".asset-cache")
	dest := filepath.Join(dir, name)
	if info, err := os.Stat(dest); err == nil && info.Mode().IsRegular() && info.Size() <= limit {
		return dest, nil
	}
	file, err := a.Open(ctx, name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > limit {
		return "", errors.New("asset exceeds image limit")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(dir, ".asset-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	n, copyErr := io.Copy(temp, io.LimitReader(file, limit+1))
	closeErr := temp.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n > limit || n != info.Size() {
		return "", errors.New("incomplete image asset")
	}
	if err = os.Rename(temp.Name(), dest); err != nil {
		return "", err
	}
	return dest, nil
}

func (a *Assets) Open(ctx context.Context, name string) (AssetFile, error) {
	key, err := a.key(name)
	if err != nil {
		return nil, err
	}
	info, err := a.Stat(ctx, name)
	if err != nil {
		return nil, err
	}
	if !a.cloud() {
		return os.Open(filepath.Join(a.Directory, name))
	}
	b, err := a.remote()
	if err != nil {
		return nil, err
	}
	return &remoteFile{ctx: ctx, bucket: b, key: key, info: info}, nil
}

type remoteFile struct {
	ctx    context.Context
	bucket *oss.Bucket
	key    string
	info   os.FileInfo
	offset int64
	body   io.ReadCloser
}

func (f *remoteFile) Stat() (os.FileInfo, error) { return f.info, nil }
func (f *remoteFile) Close() error {
	if f.body != nil {
		err := f.body.Close()
		f.body = nil
		return err
	}
	return nil
}
func (f *remoteFile) Seek(offset int64, whence int) (int64, error) {
	next := offset
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		next = f.offset + offset
	case io.SeekEnd:
		next = f.info.Size() + offset
	default:
		return 0, errors.New("invalid seek")
	}
	if next < 0 {
		return 0, errors.New("negative seek")
	}
	if next != f.offset {
		_ = f.Close()
		f.offset = next
	}
	return next, nil
}
func (f *remoteFile) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if f.offset >= f.info.Size() {
		return 0, io.EOF
	}
	if f.body == nil {
		body, err := f.bucket.GetObject(f.key, oss.Range(f.offset, f.info.Size()-1), oss.WithContext(f.ctx))
		if err != nil {
			return 0, cloudError(err)
		}
		f.body = body
	}
	n, err := f.body.Read(p)
	f.offset += int64(n)
	return n, err
}
func (a *Assets) Serve(w http.ResponseWriter, r *http.Request, name string) {
	file, err := a.Open(r.Context(), name)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, os.ErrNotExist) || !validName(name) {
			status = 404
		}
		http.Error(w, http.StatusText(status), status)
		return
	}
	defer file.Close()
	info, _ := file.Stat()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, info.ModTime(), file)
}
