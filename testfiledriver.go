package sftp

import (
	"io"
	"io/ioutil"
	"os"
)

type TestFileDriver struct {
	//s3 *s3.S3
	s3       S3
	bucket   string
	homePath string
}

func (d TestFileDriver) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (d TestFileDriver) ListDir(path string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(path)
	
	/*

		prefix, err := translatePath(d.homePath, path)
		if err != nil {
			return nil, err
		}
		objects, err := d.s3.ListObjectsV2(&s3.ListObjectsV2Input{
			Bucket:    aws.String(d.bucket),
			Prefix:    aws.String(prefix),
			Delimiter: aws.String("/"),
		})
		if err != nil {
			return nil, err
		}
		files := []os.FileInfo{}
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
				name: strings.TrimSuffix(strings.TrimPrefix(*o.Prefix, prefix), "/"),
				mode: os.ModeDir,
			})
		}
		return files, nil*/
}

func (d TestFileDriver) DeleteDir(path string) error {
	return os.Remove(path)
}

func (d TestFileDriver) DeleteFile(path string) error {
	return os.Remove(path)
}

func (d TestFileDriver) Rename(oldpath string, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (d TestFileDriver) MakeDir(path string) error {
	return os.Mkdir(path, 0755)
}

func (d TestFileDriver) GetFile(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	return f, err
}

func (d TestFileDriver) PutFile(path string, r io.Reader) error {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, bytes, 0755)
}
