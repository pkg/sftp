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
	homePath string
}

func (d S3Driver) Stat(path string) (os.FileInfo, error) {
	localPath, err := translatePath(d.homePath, path)
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
	prefix, err := translatePath(d.homePath, path)
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
	translatedPath, err := translatePath(d.homePath, path)
	if err != nil {
		return err
	}
	_, err = d.s3.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(translatedPath),
	})
	return err
}

func (d S3Driver) DeleteFile(path string) error {
	translatedPath, err := translatePath(d.homePath, path)
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
	translatedOldpath, err := translatePath(d.homePath, oldpath)
	if err != nil {
		return err
	}
	translatedNewpath, err := translatePath(d.homePath, newpath)
	if err != nil {
		return err
	}

	if _, err := d.s3.CopyObject(&s3.CopyObjectInput{
		Bucket:     aws.String(d.bucket),
		CopySource: aws.String(d.bucket + "/" + translatedOldpath),
		Key:        &translatedNewpath,
	}); err != nil {
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
	localPath, err := translatePath(d.homePath, path)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(localPath, "/") {
		localPath += "/"
	}

	_, err = d.s3.PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(d.bucket),
		Key:                  aws.String(localPath),
		ServerSideEncryption: aws.String("AES256"),
		Body:                 bytes.NewReader([]byte{}),
	})
	return err
}

func (d S3Driver) GetFile(path string) (io.ReadCloser, error) {
	localPath, err := translatePath(d.homePath, path)
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
	return obj.Body, nil
}

func (d S3Driver) PutFile(path string, r io.Reader) error {
	localPath, err := translatePath(d.homePath, path)
	if err != nil {
		return err
	}

	rawData, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	_, err = d.s3.PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(d.bucket),
		Key:                  aws.String(localPath),
		ServerSideEncryption: aws.String("AES256"),
		Body:                 bytes.NewReader(rawData),
	})
	return err
}

// translatePath takes in a home directory prefix, and a path to add to it, and returns a cleaned and validated path
// that represents the two joined together. It will resolve things like '..' while disallowing the prefix to be escaped
// from. It also preserves a single trailing slash if one is present, so it can be used on both directories and files.
func translatePath(prefix, path string) (s3Path string, err error) {
	cleanPath := filepath.Clean("/" + prefix + filepath.Clean("/"+path))
	// For some reason, filepath.Clean drops trailing /'s, so if there was one we have to put it back
	if strings.HasSuffix(path, "/") {
		cleanPath += "/"
	}
	return strings.TrimLeft(cleanPath, "/"), nil
}

func NewS3Driver(bucket, homePath, region, awsAccessKeyID, awsSecretKey string) *S3Driver {
	config := aws.NewConfig().
		WithRegion(region).
		WithCredentials(credentials.NewStaticCredentials(awsAccessKeyID, awsSecretKey, ""))
	s3 := s3.New(session.New(), config)
	return &S3Driver{
		s3:       s3,
		bucket:   bucket,
		homePath: homePath,
	}
}
