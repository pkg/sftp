package sftp

import (
	"context"
	"errors"
	"testing"
)

func TestReadDirWithLimits(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		packets              [][]string
		maxEntries, maxBytes int
		wantEntries          int
		wantLimit            bool
	}{
		{"entry limit", [][]string{{"a", "b", "c"}}, 2, 4096, 0, true},
		{"cumulative entries", [][]string{{"a"}, {"b"}, {"c"}}, 2, 4096, 0, true},
		{"byte limit", [][]string{{"a"}}, 100, 21, 0, true},
		{"cumulative bytes", [][]string{{"a"}, {"b"}}, 100, 43, 0, true},
		{"exact limits", [][]string{{"a"}}, 1, 22, 1, false},
		{"multiple packets", [][]string{{"a"}, {"b"}}, 2, 44, 2, false},
		{"empty directory", nil, 1, 1, 0, false},
		{"dot entries count", [][]string{{".", "..", "a"}}, 2, 4096, 0, true},
		{"dot entries skipped", [][]string{{".", "..", "a"}}, 3, 4096, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			packet := 0
			closed := make(chan struct{}, 1)
			client := startScriptedClient(t, func(typ fxp, id uint32) (fxp, []byte, bool) {
				switch typ {
				case sshFxpOpendir:
					return sshFxpHandle, marshalString(marshalUint32(nil, id), "dir"), true
				case sshFxpReaddir:
					if packet == len(tc.packets) {
						return sshFxpStatus, statusBody(marshalUint32(nil, id), sshFxEOF), true
					}
					names := tc.packets[packet]
					packet++
					body := marshalUint32(marshalUint32(nil, id), uint32(len(names)))
					for _, name := range names {
						body = dirEntry(body, name)
					}
					return sshFxpName, body, true
				case sshFxpClose:
					closed <- struct{}{}
					return sshFxpStatus, statusBody(marshalUint32(nil, id), sshFxOk), true
				}
				return 0, nil, false
			})
			defer client.Close()
			entries, err := client.ReadDirWithLimits(context.Background(), "/", tc.maxEntries, tc.maxBytes)
			if tc.wantLimit {
				if !errors.Is(err, ErrListingLimit) || entries != nil {
					t.Fatalf("got %v, %v, want nil, ErrListingLimit", entries, err)
				}
			} else if err != nil || len(entries) != tc.wantEntries {
				t.Fatalf("got %d entries, %v, want %d entries", len(entries), err, tc.wantEntries)
			}
			select {
			case <-closed:
			default:
				t.Fatal("directory handle was not closed")
			}
		})
	}
}

func TestReadDirWithLimitsRejectsInvalidLimitsBeforeConnecting(t *testing.T) {
	for _, limits := range [][2]int{{0, 1}, {1, 0}, {-1, 1}, {1, -1}} {
		var client *Client
		if _, err := client.ReadDirWithLimits(context.Background(), "/", limits[0], limits[1]); err == nil {
			t.Fatalf("accepted limits %v", limits)
		}
	}
}

func TestReadDirWithLimitsChecksCountBeforeDecodingEntries(t *testing.T) {
	client := startScriptedClient(t, func(typ fxp, id uint32) (fxp, []byte, bool) {
		switch typ {
		case sshFxpOpendir:
			return sshFxpHandle, marshalString(marshalUint32(nil, id), "dir"), true
		case sshFxpReaddir:
			return sshFxpName, marshalUint32(marshalUint32(nil, id), 100001), true
		case sshFxpClose:
			return sshFxpStatus, statusBody(marshalUint32(nil, id), sshFxOk), true
		}
		return 0, nil, false
	})
	defer client.Close()
	entries, err := client.ReadDirWithLimits(context.Background(), "/", 100000, 4096)
	if !errors.Is(err, ErrListingLimit) || entries != nil {
		t.Fatalf("got %v, %v, want nil, ErrListingLimit", entries, err)
	}
}

func TestReadDirWithLimitsHonorsCancellation(t *testing.T) {
	client := startScriptedClient(t, func(typ fxp, id uint32) (fxp, []byte, bool) {
		return 0, nil, false
	})
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.ReadDirWithLimits(ctx, "/", 10, 4096); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
