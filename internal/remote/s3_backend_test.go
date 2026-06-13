package remote

import "testing"

func TestParseS3EndpointOptionsFromURI(t *testing.T) {
	bucket, prefix, options, err := parseS3Endpoint("s3://pinax-test/prefix/path?endpoint=http%3A%2F%2F10.10.1.102%3A9010&region=us-east-1&path_style=true&profile=minio")
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	if bucket != "pinax-test" || prefix != "prefix/path" {
		t.Fatalf("bucket=%q prefix=%q", bucket, prefix)
	}
	if options.EndpointURL != "http://10.10.1.102:9010" || options.Region != "us-east-1" || options.Profile != "minio" || !options.PathStyle {
		t.Fatalf("options=%#v", options)
	}
}

func TestParseS3EndpointDefaultsToPathStyleForCustomEndpoint(t *testing.T) {
	_, _, options, err := parseS3Endpoint("s3://pinax-test/prefix?endpoint=http%3A%2F%2F10.10.1.102%3A9010")
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	if !options.PathStyle {
		t.Fatalf("custom endpoint should default to path style: %#v", options)
	}
}
