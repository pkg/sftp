package sftp

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type S3 interface {
	ListObjectsV2(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error)
	DeleteObject(input *s3.DeleteObjectInput) (*s3.DeleteObjectOutput, error)
	CopyObject(input *s3.CopyObjectInput) (*s3.CopyObjectOutput, error)
	PutObject(input *s3.PutObjectInput) (*s3.PutObjectOutput, error)
	GetObject(input *s3.GetObjectInput) (*s3.GetObjectOutput, error)
}

type S3Driver struct {
	s3       S3
	bucket   string
	prefix   string
	homePath string
	kmsKeyID *string
	lg       Logger
}

func (d S3Driver) Stat(path string) (os.FileInfo, error) {
	localPath, err := TranslatePath(d.prefix, d.homePath, path)
	if err != nil {
		return nil, err
	}

	resp, err := d.s3.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket:  aws.String(d.bucket),
		Prefix:  aws.String(localPath),
		MaxKeys: aws.Int64(1),
	})
	if err != nil {
		return nil, err
	}

	if resp.Contents == nil || *resp.KeyCount == 0 {
		return nil, os.ErrNotExist
	}

	info := &fileInfo{
		name:  localPath,
		mode:  os.ModePerm,
		size:  *resp.Contents[0].Size,
		mtime: *resp.Contents[0].LastModified,
	}
	if strings.HasSuffix(*resp.Contents[0].Key, "/") {
		info.name = strings.TrimRight(info.name, "/")
		info.mode = os.ModeDir
	}
	return info, nil
}

func (d S3Driver) ListDir(path string) ([]os.FileInfo, error) {
	prefix, err := TranslatePath(d.prefix, d.homePath, path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	var nextContinuationToken *string
	files := []os.FileInfo{}
	for {
		objects, err := d.s3.ListObjectsV2(&s3.ListObjectsV2Input{
			Bucket:            aws.String(d.bucket),
			Prefix:            aws.String(prefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: nextContinuationToken,
		})
		if err != nil {
			return nil, err
		}

		for _, o := range objects.Contents {
			if *o.Key == prefix {
				continue
			}
			files = append(files, &fileInfo{
				name:  strings.TrimPrefix(*o.Key, prefix),
				size:  *o.Size,
				mtime: *o.LastModified,
			})
		}
		for _, o := range objects.CommonPrefixes {
			files = append(files, &fileInfo{
				name:  strings.TrimSuffix(strings.TrimPrefix(*o.Prefix, prefix), "/"),
				size:  4096,
				mtime: time.Unix(1, 0),
				mode:  os.ModeDir,
			})
		}

		if !*objects.IsTruncated {
			return files, nil
		}
		nextContinuationToken = objects.NextContinuationToken
	}
}

func (d S3Driver) DeleteDir(path string) error {
	translatedPath, err := TranslatePath(d.prefix, d.homePath, path)
	if err != nil {
		return err
	}

	// s3 DeleteObject needs a trailing slash for directories
	directoryPath := translatedPath
	if !strings.HasSuffix(translatedPath, "/") {
		directoryPath = translatedPath + "/"
	}

	_, err = d.s3.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(directoryPath),
	})
	return err
}

func (d S3Driver) DeleteFile(path string) error {
	translatedPath, err := TranslatePath(d.prefix, d.homePath, path)
	if err != nil {
		return err
	}
	_, err = d.s3.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(translatedPath),
	})
	return err
}

func (d S3Driver) Rename(oldpath string, newpath string) error {
	translatedOldpath, err := TranslatePath(d.prefix, d.homePath, oldpath)
	if err != nil {
		return err
	}
	translatedNewpath, err := TranslatePath(d.prefix, d.homePath, newpath)
	if err != nil {
		return err
	}
	input := &s3.CopyObjectInput{
		Bucket:     aws.String(d.bucket),
		CopySource: aws.String(d.bucket + "/" + translatedOldpath),
		Key:        &translatedNewpath,
	}
	if d.kmsKeyID == nil {
		input.ServerSideEncryption = aws.String("AES256")
	} else {
		input.ServerSideEncryption = aws.String("aws:kms")
		input.SSEKMSKeyId = aws.String(*d.kmsKeyID)
	}
	if _, err := d.s3.CopyObject(input); err != nil {
		return err
	}

	if _, err = d.s3.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    &translatedOldpath,
	}); err != nil {
		return err
	}

	return nil
}

func (d S3Driver) MakeDir(path string) error {
	localPath, err := TranslatePath(d.prefix, d.homePath, path)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(localPath, "/") {
		localPath += "/"
	}
	input := &s3.PutObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(localPath),
		Body:   bytes.NewReader([]byte{}),
	}
	if d.kmsKeyID == nil {
		input.ServerSideEncryption = aws.String("AES256")
	} else {
		input.ServerSideEncryption = aws.String("aws:kms")
		input.SSEKMSKeyId = aws.String(*d.kmsKeyID)
	}
	_, err = d.s3.PutObject(input)
	return err
}

func (d S3Driver) GetFile(path string) (io.ReadCloser, error) {
	localPath, err := TranslatePath(d.prefix, d.homePath, path)
	if err != nil {
		return nil, err
	}
	obj, err := d.s3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(localPath),
	})
	if err != nil {
		return nil, err
	}
	if d.lg != nil {
		d.lg.InfoD("s3Driver-get-file-success", meta{
			"district_id":     d.bucket,
			"method":          "GET",
			"path":            localPath,
			"file_bytes_size": obj.ContentLength,
		})
	}
	return obj.Body, nil
}

func (d S3Driver) PutFile(path string, r io.Reader) error {
	localPath, err := TranslatePath(d.prefix, d.homePath, path)
	if err != nil {
		return err
	}

	rawData, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	input := &s3.PutObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(localPath),
		Body:   bytes.NewReader(rawData),
	}
	if d.kmsKeyID == nil {
		input.ServerSideEncryption = aws.String("AES256")
	} else {
		input.ServerSideEncryption = aws.String("aws:kms")
		input.SSEKMSKeyId = aws.String(*d.kmsKeyID)
	}
	_, err = d.s3.PutObject(input)
	if err != nil {
		return err
	}
	if d.lg != nil {
		d.lg.InfoD("s3Driver-put-file-success", meta{
			"district_id":     d.bucket,
			"method":          "PUT",
			"path":            localPath,
			"file_bytes_size": input.ContentLength,
		})
	}
	return nil
}

func (d S3Driver) RealPath(path string) string {
	result, _ := TranslatePath("/", d.homePath, path)
	return "/" + result
}

// translatePath takes in a S3 root prefix, a home directory, and either an absolute or relative path to append, and returns a cleaned and validated path.
// It will resolve things like '..' while disallowing the prefix to be escaped.
// It also preserves a single trailing slash if one is present, so it can be used on both directories and files.
func TranslatePath(prefix, home, path string) (string, error) {
	if path == "" {
		return filepath.Clean("/" + prefix + "/" + home), nil
	}

	var cleanPath string
	if strings.HasPrefix(path, "/") {
		cleanPath = filepath.Clean(prefix + path)
		if !strings.HasPrefix(cleanPath, prefix) {
			cleanPath = prefix
		}
	} else {
		cleanPath = filepath.Clean("/" + prefix + "/" + home + filepath.Clean("/"+path))
	}

	// For some reason, filepath.Clean drops trailing /'s, so if there was one we have to put it back
	if strings.HasSuffix(path, "/") {
		cleanPath += "/"
	}
	return strings.TrimLeft(cleanPath, "/"), nil
}

// NewS3Driver creates a new S3Driver with the AWS credentials and S3 parameters.
// bucket: name of S3 bucket
// prefix: key within the S3 bucket, if applicable
// homePath: default home directory for user (can be different from prefix)
func NewS3Driver(bucket, prefix, homePath, region, awsAccessKeyID, awsSecretKey, awsToken string, kmsKeyID *string, lg Logger) *S3Driver {
	config := aws.NewConfig().
		WithRegion(region).
		WithCredentials(credentials.NewStaticCredentials(awsAccessKeyID, awsSecretKey, awsToken))
	s3 := s3.New(session.New(), config)
	return &S3Driver{
		s3:       s3,
		bucket:   bucket,
		prefix:   prefix,
		homePath: homePath,
		kmsKeyID: kmsKeyID,
		lg:       lg,
	}
}
