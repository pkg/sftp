package sftp

import "github.com/pkg/errors"

type SSHFxpDataPacket struct {
	ID     uint32
	Length uint32
	Data   []byte
}

func (p SSHFxpDataPacket) Id() uint32 { return p.ID }

func (p SSHFxpDataPacket) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_DATA}
	b = marshalUint32(b, p.ID)
	b = marshalUint32(b, p.Length)
	b = append(b, p.Data[:p.Length]...)
	return b, nil
}

func (p *SSHFxpDataPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Length, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if uint32(len(b)) < p.Length {
		return errors.New("truncated packet")
	}

	p.Data = make([]byte, p.Length)
	copy(p.Data, b)
	return nil
}
