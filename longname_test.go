package sftp

import (
	"os"
	"os/user"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	typeDirectory = "d"
	typeFile      = "[^d]"
)

type idLookup struct{}

func (idLookup) LookupUserName(uid string) string {
	u, err := user.LookupId(uid)
	if err != nil {
		return uid
	}

	return u.Username
}

func (idLookup) LookupGroupName(gid string) string {
	g, err := user.LookupGroupId(gid)
	if err != nil {
		return gid
	}

	return g.Name
}

func TestRunLsWithExamplesDirectory(t *testing.T) {
	path := "localfs"
	item, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	result := FormatLongname(item, nil)
	FormatLongnameTestHelper(t, result, typeDirectory, path)
}

func TestRunLsWithLicensesFile(t *testing.T) {
	path := "LICENSE"
	item, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	result := FormatLongname(item, nil)
	FormatLongnameTestHelper(t, result, typeFile, path)
}

func TestRunLsWithExamplesDirectoryWithOSLookup(t *testing.T) {
	path := "localfs"
	item, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	result := FormatLongname(item, idLookup{})
	FormatLongnameTestHelper(t, result, typeDirectory, path)
}

func TestRunLsWithLicensesFileWithOSLookup(t *testing.T) {
	path := "LICENSE"
	item, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	result := FormatLongname(item, idLookup{})
	FormatLongnameTestHelper(t, result, typeFile, path)
}

/*
The format of the `longname' field is unspecified by this protocol.
It MUST be suitable for use in the output of a directory listing
command (in fact, the recommended operation for a directory listing
command is to simply display this data).  However, clients SHOULD NOT
attempt to parse the longname field for file attributes; they SHOULD
use the attrs field instead.

	The recommended format for the longname field is as follows:

	    -rwxr-xr-x   1 mjos     staff      348911 Mar 25 14:29 t-filexfer
	    1234567890 123 12345678 12345678 12345678 123456789012

Here, the first line is sample output, and the second field indicates
widths of the various fields.  Fields are separated by spaces.  The
first field lists file permissions for user, group, and others; the
second field is link count; the third field is the name of the user
who owns the file; the fourth field is the name of the group that
owns the file; the fifth field is the size of the file in bytes; the
sixth field (which actually may contain spaces, but is fixed to 12
characters) is the file modification time, and the seventh field is
the file name.  Each field is specified to be a minimum of certain
number of character positions (indicated by the second line above),
but may also be longer if the data does not fit in the specified
length.

	The SSH_FXP_ATTRS response has the following format:

	    uint32     id
	    ATTRS      attrs

where `id' is the request identifier, and `attrs' is the returned
file attributes as described in Section “File Attributes”.

N.B.: FileZilla does parse this ls formatting, and so not rendering it
on any particular GOOS/GOARCH can cause compatibility issues with this client.
*/
func FormatLongnameTestHelper(t *testing.T, result, expectedType, path string) {
	// using regular expressions to make tests work on all systems
	// a virtual file system (like afero) would be needed to mock valid filesystem checks
	// expected layout is:
	// drwxr-xr-x   8 501      20            272 Aug  9 19:46 localfs

	t.Log(result)

	sparce := strings.Split(result, " ")

	var fields []string
	for _, field := range sparce {
		if field == "" {
			continue
		}

		fields = append(fields, field)
	}

	perms, linkCnt, user, group, size := fields[0], fields[1], fields[2], fields[3], fields[4]
	dateTime := strings.Join(fields[5:8], " ")
	filename := fields[8]

	if runtime.GOOS == "zos" {
		// User and Group are always only uppercase characters on z/OS
		user = strings.ToLower(user)
		group = strings.ToLower(group)
	}

	// permissions (len 10, "drwxr-xr-x")
	const (
		rwxs = "[-r][-w][-xsS]"
		rwxt = "[-r][-w][-xtT]"
	)
	if ok, err := regexp.MatchString("^"+expectedType+rwxs+rwxs+rwxt+"$", perms); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("FormatLongname(%q): permission field mismatch, expected dir, got: %#v, err: %#v", path, perms, err)
	}

	// link count (len 3, number)
	const (
		number = "(?:[0-9]+)"
	)
	if ok, err := regexp.MatchString("^"+number+"$", linkCnt); !ok && linkCnt != "?" {
		// OpenSSH itself uses '?' as the link count, when it cannot get the link count otherwise.
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("FormatLongname(%q): link count field mismatch, got: %#v, err: %#v", path, linkCnt, err)
	}

	// username / uid (len 8, number or string)
	const (
		name = "(?:[A-Za-z_][-A-Z.a-z0-9_]*)"
	)
	if ok, err := regexp.MatchString("^(?:"+number+"|"+name+")+$", user); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("FormatLongname(%q): username / uid mismatch, expected user, got: %#v, err: %#v", path, user, err)
	}

	// groupname / gid (len 8, number or string)
	if ok, err := regexp.MatchString("^(?:"+number+"|"+name+")+$", group); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("FormatLongname(%q): groupname / gid mismatch, expected group, got: %#v, err: %#v", path, group, err)
	}

	// filesize (len 8)
	if ok, err := regexp.MatchString("^"+number+"$", size); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("FormatLongname(%q): filesize field mismatch, expected size in bytes, got: %#v, err: %#v", path, size, err)
	}

	// mod time (len 12, e.g. Aug  9 19:46)
	_, err := time.Parse("Jan 2 15:04", dateTime)
	if err != nil {
		_, err = time.Parse("Jan 2 2006", dateTime)
		if err != nil {
			t.Errorf("FormatLongname.dateTime = %#v should match `Jan 2 15:04` or `Jan 2 2006`: %+v", dateTime, err)
		}
	}

	// filename
	if path != filename {
		t.Errorf("FormatLongname.filename = %#v, expected: %#v", filename, path)
	}
}
