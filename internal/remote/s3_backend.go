package remote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

func init() {
	Register("s3", func(ctx context.Context, endpoint string) (BlobStore, error) {
		bucket, prefix, options, err := parseS3Endpoint(endpoint)
		if err != nil {
			return nil, err
		}
		return NewS3BackendWithOptions(ctx, bucket, prefix, options)
	})
}

type S3BackendOptions struct {
	EndpointURL string
	Region      string
	Profile     string
	PathStyle   bool
	PathMode    string
	API         string
	// CredentialsProvider, when non-nil, is the explicit AWS SDK credentials
	// provider used to authenticate S3 requests (for example, a StaticCredentialsProvider
	// built from a repository-encrypted bundle). When nil, the SDK shared
	// profile / default credential chain is used (existing device-profile behavior).
	CredentialsProvider aws.CredentialsProvider
}

func parseS3Endpoint(endpoint string) (string, string, S3BackendOptions, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", S3BackendOptions{}, err
	}
	options := S3BackendOptions{}
	q := u.Query()
	options.EndpointURL = strings.TrimSpace(q.Get("endpoint"))
	options.Region = strings.TrimSpace(q.Get("region"))
	options.Profile = strings.TrimSpace(q.Get("profile"))
	options.PathMode = strings.TrimSpace(q.Get("path"))
	options.API = strings.TrimSpace(q.Get("api"))
	pathStyle := s3PathStyleFromQuery(options.EndpointURL, q)
	options.PathStyle = pathStyle
	return u.Host, strings.TrimPrefix(u.Path, "/"), options, nil
}

func s3PathStyleFromQuery(endpointURL string, q url.Values) bool {
	style := strings.ToLower(strings.TrimSpace(firstNonEmpty(q.Get("addressing_style"), q.Get("path"))))
	switch style {
	case "virtual", "virtual-hosted", "virtual_hosted", "host", "hosted":
		return false
	case "path", "path-style", "path_style", "on", "true":
		return true
	}
	if strings.EqualFold(q.Get("path_style"), "true") {
		return true
	}
	return defaultS3PathStyle(endpointURL)
}

func defaultS3PathStyle(endpointURL string) bool {
	endpointURL = strings.ToLower(strings.TrimSpace(endpointURL))
	if endpointURL == "" {
		return false
	}
	if strings.Contains(endpointURL, ".myqcloud.com") || strings.Contains(endpointURL, ".myqcloud.com.cn") {
		return false
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

type S3Backend struct {
	client *s3.Client
	bucket string
	prefix string
}

func NewS3BackendWithOptions(ctx context.Context, bucket string, prefix string, options S3BackendOptions) (*S3Backend, error) {
	loadOptions := make([]func(*config.LoadOptions) error, 0, 3)
	if options.Region != "" {
		loadOptions = append(loadOptions, config.WithRegion(options.Region))
	}
	if options.Profile != "" {
		loadOptions = append(loadOptions, config.WithSharedConfigProfile(options.Profile))
	}
	// When an explicit credentials provider is supplied (repository-encrypted
	// mode), inject it directly so the SDK does not consult the shared profile
	// chain or device-local env — preventing accidental cross-account access.
	if options.CredentialsProvider != nil {
		loadOptions = append(loadOptions, config.WithCredentialsProvider(options.CredentialsProvider))
	}
	cfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if options.EndpointURL != "" {
			o.BaseEndpoint = aws.String(options.EndpointURL)
		}
		o.UsePathStyle = options.PathStyle
		// S3-compatible services (e.g. Tencent COS) rarely return x-amz-checksum-*
		// headers, which the SDK's default WhenSupported policy turns into a WARN
		// per object. Only validate checksums when the operation requires it.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	return &S3Backend{
		client: client,
		bucket: bucket,
		prefix: prefix,
	}, nil
}

func (s *S3Backend) SupportsConditionalWrites() bool { return true }

func (s *S3Backend) Get(ctx context.Context, key string) ([]byte, string, error) {
	fullKey := s.prefix + key
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchKey" {
			return nil, "", ErrObjectNotFound
		}
		return nil, "", err
	}
	defer func() { _ = out.Body.Close() }()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, "", err
	}
	rev := ""
	if out.ETag != nil {
		rev = strings.Trim(*out.ETag, "\"")
	}
	return data, rev, nil
}

func (s *S3Backend) Put(ctx context.Context, key string, data []byte, baseRev string) (string, error) {
	fullKey := s.prefix + key
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
		Body:   bytes.NewReader(data),
	}
	if baseRev == CreateIfAbsentRevision {
		in.IfNoneMatch = aws.String("*")
	} else if baseRev != "" {
		in.IfMatch = aws.String("\"" + baseRev + "\"")
	}
	out, err := s.client.PutObject(ctx, in)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "PreconditionFailed" || apiErr.ErrorCode() == "ConditionalRequestConflict") {
			return "", ErrConflict
		}
		return "", err
	}
	rev := ""
	if out.ETag != nil {
		rev = strings.Trim(*out.ETag, "\"")
	}
	return rev, nil
}

func (s *S3Backend) Stat(ctx context.Context, key string) (string, error) {
	fullKey := s.prefix + key
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NotFound" {
			return "", ErrObjectNotFound
		}
		return "", err
	}
	rev := ""
	if out.ETag != nil {
		rev = strings.Trim(*out.ETag, "\"")
	}
	return rev, nil
}

func (s *S3Backend) Delete(ctx context.Context, key string) error {
	fullKey := s.prefix + key
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fullKey),
	})
	return err
}

// List returns objects with the given prefix.
func (s *S3Backend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	fullPrefix := s.prefix + prefix
	var objects []ObjectInfo
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(fullPrefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			key := strings.TrimPrefix(*obj.Key, s.prefix)
			info := ObjectInfo{Key: key}
			// S3-compatible stores (notably Tencent COS) may omit optional
			// list fields; dereferencing them unguarded panics the sync.
			if obj.Size != nil {
				info.Size = *obj.Size
			}
			if obj.ETag != nil {
				info.Revision = *obj.ETag
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			objects = append(objects, info)
		}
	}
	return objects, nil
}

// Exists checks if an object exists.
func (s *S3Backend) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Stat(ctx, key)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// BatchStat returns revisions for multiple keys.
func (s *S3Backend) BatchStat(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		rev, err := s.Stat(ctx, key)
		if err == nil {
			result[key] = rev
		}
	}
	return result, nil
}
