package app

import (
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestBackendS3OptionsUseProfileFields(t *testing.T) {
	t.Parallel()
	profile := domain.BackendProfile{
		Kind:     domain.BackendS3,
		Bucket:   "notes",
		Region:   "us-east-1",
		Endpoint: "http://127.0.0.1:9000",
		Profile:  "work",
	}

	options := backendS3Options(profile)
	if options.Region != "us-east-1" || options.EndpointURL != "http://127.0.0.1:9000" || options.Profile != "work" || !options.PathStyle {
		t.Fatalf("s3 options did not preserve backend profile fields: %#v", options)
	}
}

func TestBackendS3OptionsUseVirtualHostStyleForTencentCOS(t *testing.T) {
	t.Parallel()
	profile := domain.BackendProfile{
		Kind:     domain.BackendS3,
		Bucket:   "notes",
		Region:   "ap-guangzhou",
		Endpoint: "https://cos.ap-guangzhou.myqcloud.com",
		Profile:  "tencent-cos-pinax",
	}

	options := backendS3Options(profile)
	if options.PathStyle {
		t.Fatalf("tencent cos should use virtual-hosted style: %#v", options)
	}
}
