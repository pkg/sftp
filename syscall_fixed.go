// Go defines S_IFMT on windows, plan9 and js/wasm as 0x1f000 instead of
// 0xf000. None of the the other S_IFxyz values include the "1" (in 0x1f000)
// which prevents them from matching the bitmask.

package sftp

import sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"

const S_IFMT = uint32(sshfx.ModeType)
