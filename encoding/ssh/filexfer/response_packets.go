package sshfx

import (
	"errors"
)

// StatusPacket defines the SSH_FXP_STATUS packet.
//
// Specified in https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt#section-7
type StatusPacket struct {
	StatusCode   Status
	ErrorMessage string
	LanguageTag  string
}

// Error makes StatusPacket an error type.
func (p *StatusPacket) Error() string {
	if p.ErrorMessage == "" {
		return "sftp: " + p.StatusCode.String()
	}

	return "sftp: " + p.StatusCode.String() + ": " + p.ErrorMessage
}

// Is returns true if the status packet is the same error as target.
// If target is a *StatusPacket, then we return true if all fields are equal.
// Otherwise, we return true if [Status.Is] of the status code returns true for the same target.
func (p *StatusPacket) Is(target error) bool {
	var status *StatusPacket
	if errors.As(target, &status) {
		return *p == *status
	}

	return p.StatusCode.Is(target)
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *StatusPacket) Type() PacketType {
	return PacketTypeStatus
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *StatusPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + uint32(error/status code) + string(error message) + string(language tag)
	return 1 + 4 + 4 + 4 + len(p.ErrorMessage) + 4 + len(p.LanguageTag)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *StatusPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeStatus, reqid)
	buf.AppendUint32(uint32(p.StatusCode))
	buf.AppendString(p.ErrorMessage)
	buf.AppendString(p.LanguageTag)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *StatusPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = StatusPacket{
		StatusCode:   Status(buf.ConsumeUint32()),
		ErrorMessage: buf.ConsumeString(),
		LanguageTag:  buf.ConsumeString(),
	}

	return buf.Err
}

// HandlePacket defines the SSH_FXP_HANDLE packet.
type HandlePacket struct {
	Handle string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *HandlePacket) Type() PacketType {
	return PacketTypeHandle
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *HandlePacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(handle)
	return 1 + 4 + 4 + len(p.Handle)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *HandlePacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeHandle, reqid)
	buf.AppendString(p.Handle)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *HandlePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = HandlePacket{
		Handle: buf.ConsumeString(),
	}

	return buf.Err
}

// DataPacket defines the SSH_FXP_DATA packet.
type DataPacket struct {
	Data []byte
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *DataPacket) Type() PacketType {
	return PacketTypeData
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *DataPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + bytes(data)
	return 1 + 4 + 4 + len(p.Data)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *DataPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		// exclude the data length, that will be carried in the separate payload.
		buf = NewMarshalBuffer(p.MarshalSize() - len(p.Data))
	}

	buf.StartPacket(PacketTypeData, reqid)
	buf.AppendUint32(uint32(len(p.Data)))

	return buf.Packet(p.Data)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
//
// If p.Data is already populated, and of sufficient length to hold the data,
// then this will copy the data into that byte slice.
//
// If p.Data has a length insufficient to hold the data,
// then this will make a new slice of sufficient length, and copy the data into that.
//
// This means this _does not_ alias any of the data buffer that is passed in.
func (p *DataPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = DataPacket{
		Data: buf.ConsumeBytesCopy(p.Data),
	}

	return buf.Err
}

// NamePacket defines the SSH_FXP_NAME packet.
type NamePacket struct {
	Entries []*NameEntry
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *NamePacket) Type() PacketType {
	return PacketTypeName
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *NamePacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + uint32(len(entries))
	size := 1 + 4 + 4

	for _, e := range p.Entries {
		size += e.MarshalSize()
	}

	return size
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *NamePacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeName, reqid)
	buf.AppendUint32(uint32(len(p.Entries)))

	for _, e := range p.Entries {
		e.MarshalInto(buf)
	}

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *NamePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	count, err := buf.ConsumeCount()
	if err != nil {
		return err
	}

	*p = NamePacket{
		Entries: make([]*NameEntry, 0, count),
	}

	for range count {
		var e NameEntry
		if err := e.UnmarshalFrom(buf); err != nil {
			return err
		}

		p.Entries = append(p.Entries, &e)
	}

	return buf.Err
}

// PathPseudoPacket defines a SSH_FXP_NAME packet that contains only one entry, which is a file path.
type PathPseudoPacket struct {
	Path string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *PathPseudoPacket) Type() PacketType {
	return PacketTypeName
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *PathPseudoPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) +
	size := 1 + 4 + 4 // uint32(count = 1)

	size += 4 + len(p.Path) // string(path)

	size += 4 + len("") // string(longname = "")

	size += 4 // ATTRS([0]attrs{})

	return size
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *PathPseudoPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeName, reqid)
	buf.AppendUint32(uint32(1))

	buf.AppendString(p.Path)
	buf.AppendString("")
	buf.AppendUint32(uint32(0))

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *PathPseudoPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	count, err := buf.ConsumeCount()
	if err != nil {
		return err
	}

	if count == 0 {
		return errors.New("no entry returned")
	}

	var e NameEntry
	if err := e.UnmarshalFrom(buf); err != nil {
		return err
	}

	*p = PathPseudoPacket{
		Path: e.Filename,
	}

	for range count - 1 {
		var e NameEntry
		if err := e.UnmarshalFrom(buf); err != nil {
			return err
		}
	}

	return buf.Err
}

// AttrsPacket defines the SSH_FXP_ATTRS packet.
type AttrsPacket struct {
	Attrs Attributes
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *AttrsPacket) Type() PacketType {
	return PacketTypeAttrs
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *AttrsPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + ATTRS(attrs)
	return 1 + 4 + p.Attrs.MarshalSize()
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *AttrsPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeAttrs, reqid)
	p.Attrs.MarshalInto(buf)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *AttrsPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	return p.Attrs.UnmarshalFrom(buf)
}
