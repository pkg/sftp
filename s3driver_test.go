package sftp

import (
	"testing"
)

func TestTranslatePathSimple(t *testing.T) {
	path, err := translatePath("sftp/test_user", "/some_file")
	if err != nil || path != "sftp/test_user/some_file" {
		t.FailNow()
	}

	path, err = translatePath("sftp/test_user", "/some_dir/some_file")
	if err != nil || path != "sftp/test_user/some_dir/some_file" {
		t.FailNow()
	}

	path, err = translatePath("sftp/test_user", "/some_dir/../some_other_file")
	if err != nil || path != "sftp/test_user/some_other_file" {
		t.FailNow()
	}
}

func TestTranslatePathEscaping(t *testing.T) {
	path, err := translatePath("sftp/test_user", "/some_dir/../../some_escape_attempt")
	if err != nil || path != "sftp/test_user/some_escape_attempt" {
		t.FailNow()
	}

	path, err = translatePath("sftp/test_user", "///some_dir/./../../../another_escape_attempt")
	if err != nil || path != "sftp/test_user/another_escape_attempt" {
		t.FailNow()
	}
}
