package sshfx

import (
	"io/fs"
	"path"
	"time"
)

// Attributes related flags.
const (
	AttrSize        = 1 << iota // SSH_FILEXFER_ATTR_SIZE
	AttrUIDGID                  // SSH_FILEXFER_ATTR_UIDGID
	AttrPermissions             // SSH_FILEXFER_ATTR_PERMISSIONS
	AttrACModTime               // SSH_FILEXFER_ACMODTIME

	AttrExtended = 1 << 31 // SSH_FILEXFER_ATTR_EXTENDED
)

// Attributes defines the file attributes type defined in draft-ietf-secsh-filexfer-02
//
// Defined in: https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt#section-5
type Attributes struct {
	Flags uint32

	// AttrSize
	Size uint64

	// AttrUIDGID
	UID uint32
	GID uint32

	// AttrPermissions
	Permissions FileMode

	// AttrACModTime
	ATime uint32
	MTime uint32

	// AttrExtended
	Extended ExtendedAttributes
}

// GetSize returns the size field.
// It returns zero if the field is not set.
func (a *Attributes) GetSize() (size uint64) {
	if a == nil {
		return 0
	}

	return a.Size
}

// HasSize returns true if the Attributes has a size defined.
func (a *Attributes) HasSize() bool {
	if a == nil {
		return false
	}

	return a.Flags&AttrSize != 0
}

// SetSize sets the size field to a specific value.
func (a *Attributes) SetSize(size uint64) {
	a.Size = size
	a.Flags |= AttrSize
}

// ClearSize clears the size field, so that it is no longer set.
func (a *Attributes) ClearSize() {
	a.Size = 0
	a.Flags &^= AttrSize
}

// GetUserGroup returns the uid and gid field.
// It returns zeros if the field is not set.
func (a *Attributes) GetUserGroup() (uid, gid uint32) {
	if a == nil {
		return 0, 0
	}

	return a.UID, a.GID
}

// HasUserGroup returns true if the uid and gid fields are set.
func (a *Attributes) HasUserGroup() bool {
	if a == nil {
		return false
	}

	return a.Flags&AttrUIDGID != 0
}

// SetUserGroup sets the uid and gid fields to specific values.
func (a *Attributes) SetUserGroup(uid, gid uint32) {
	a.UID = uid
	a.GID = gid
	a.Flags |= AttrUIDGID
}

// ClearUserGroup clears the uid and gid fields, so that they are no longer set.
func (a *Attributes) ClearUserGroup() {
	a.UID = 0
	a.GID = 0
	a.Flags &^= AttrUIDGID
}

// GetPermissions returns the permissions field.
// It returns zero if the field is not set.
func (a *Attributes) GetPermissions() FileMode {
	if a == nil {
		return 0
	}

	return a.Permissions
}

// HasPermissions returns true if the permissions field is set.
func (a *Attributes) HasPermissions() bool {
	if a == nil {
		return false
	}

	return a.Flags&AttrPermissions != 0
}

// SetPermissions sets the permissions field to a specific value.
func (a *Attributes) SetPermissions(perms FileMode) {
	a.Permissions = perms
	a.Flags |= AttrPermissions
}

// ClearPermissions clears the permissions field, so that it is no longer set.
func (a *Attributes) ClearPermissions() {
	a.Permissions = 0
	a.Flags &^= AttrPermissions
}

// GetACModTime returns the atime and mtime fields.
// It returns zeros if the fields are not set.
func (a *Attributes) GetACModTime() (atime, mtime uint32) {
	return a.ATime, a.MTime
}

// HasACModTime returns true if the atime and mtime fields are set.
func (a *Attributes) HasACModTime() bool {
	return a.Flags&AttrACModTime != 0
}

// SetACModTime sets the atime and mtime fields to specific values.
func (a *Attributes) SetACModTime(atime, mtime uint32) {
	a.ATime = atime
	a.MTime = mtime
	a.Flags |= AttrACModTime
}

// ClearACModTime clears the atime and mtime fields, so that they are no longer set.
func (a *Attributes) ClearACModTime() {
	a.ATime = 0
	a.MTime = 0
	a.Flags &^= AttrACModTime
}

// GetExtended returns the extended field.
// It returns a nil slice if not set.
func (a *Attributes) GetExtended() ExtendedAttributes {
	if a == nil {
		return nil
	}

	return a.Extended
}

// GetExtendedAsMap returns the extended field converted into a map[string]string.
// Since this will deduplicate each key value to the last extended attribute specified and involves extra allocation,
// it is recommended to use [GetExtended], which returns a rich slice that can be iterated on as if it were a map.
func (a *Attributes) GetExtendedAsMap() map[string]string {
	if a == nil {
		return nil
	}

	return a.Extended.AsMap()
}

// HasExtended returns true if the extended field is set.
func (a *Attributes) HasExtended() bool {
	if a == nil {
		return false
	}

	return a.Flags&AttrExtended != 0
}

// SetExtended sets the extended field to the specific slice.
func (a *Attributes) SetExtended(exts ExtendedAttributes) {
	a.Extended = exts
	a.Flags |= AttrExtended
}

// SetExtendedFromMap sets the extended field to the specific map.
// The resulting slice will be sorted by type key value.
func (a *Attributes) SetExtendedFromMap(exts map[string]string) {
	var tmp ExtendedAttributes
	tmp.SetFromMap(exts)
	a.SetExtended(tmp)
}

// ClearExtended clears the extended field, so that it is no longer set.
func (a *Attributes) ClearExtended() {
	a.Extended = nil
	a.Flags &^= AttrExtended
}

// MarshalSize returns the number of bytes the attributes would marshal into.
func (a *Attributes) MarshalSize() int {
	length := 4

	if a.HasSize() {
		length += 8
	}

	if a.HasUserGroup() {
		length += 4 + 4
	}

	if a.HasPermissions() {
		length += 4
	}

	if a.HasACModTime() {
		length += 4 + 4
	}

	if a.HasExtended() {
		length += a.Extended.MarshalSize()
	}

	return length
}

// MarshalInto marshals the attributes onto the end of the buffer.
func (a *Attributes) MarshalInto(buf *Buffer) {
	buf.AppendUint32(a.Flags)

	if a.HasSize() {
		buf.AppendUint64(a.Size)
	}

	if a.HasUserGroup() {
		buf.AppendUint32(a.UID)
		buf.AppendUint32(a.GID)
	}

	if a.HasPermissions() {
		buf.AppendUint32(uint32(a.Permissions))
	}

	if a.HasACModTime() {
		buf.AppendUint32(a.ATime)
		buf.AppendUint32(a.MTime)
	}

	if a.HasExtended() {
		a.Extended.MarshalInto(buf)
	}
}

// MarshalBinary returns the binary encoding of attributes.
func (a *Attributes) MarshalBinary() ([]byte, error) {
	buf := NewBuffer(make([]byte, 0, a.MarshalSize()))
	a.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom unmarshals the attributes from the buffer.
func (a *Attributes) UnmarshalFrom(buf *Buffer) (err error) {
	a.Flags = buf.ConsumeUint32()

	// Short-circuit dummy attributes.
	if a.Flags == 0 {
		return buf.Err
	}

	if a.HasSize() {
		a.Size = buf.ConsumeUint64()
	}

	if a.HasUserGroup() {
		a.UID = buf.ConsumeUint32()
		a.GID = buf.ConsumeUint32()
	}

	if a.HasPermissions() {
		a.Permissions = FileMode(buf.ConsumeUint32())
	}

	if a.HasACModTime() {
		a.ATime = buf.ConsumeUint32()
		a.MTime = buf.ConsumeUint32()
	}

	if a.HasExtended() {
		a.Extended.UnmarshalFrom(buf)
	}

	return buf.Err
}

// UnmarshalBinary decodes the binary encoding of the attributes.
func (a *Attributes) UnmarshalBinary(data []byte) error {
	return a.UnmarshalFrom(NewBuffer(data))
}

// ExtendedAttributes is a rich slice, which provides iterators and accesssors that operate as if it were a map:
//
//	for typ, data := range attrs.GetExtended().Seq2() {
//		// Do something with the typ and data here.
//	}
type ExtendedAttributes []ExtendedAttribute

// MarshalSize returns the number of bytes the extended attributes would marshal into.
func (a ExtendedAttributes) MarshalSize() int {
	length := 4

	for _, ext := range a {
		length += ext.MarshalSize()
	}

	return length
}

// MarshalInto marshals the extended attributes onto the end of the buffer.
func (a ExtendedAttributes) MarshalInto(buf *Buffer) {
	buf.AppendUint32(uint32(len(a)))

	for _, ext := range a {
		ext.MarshalInto(buf)
	}
}

// MarshalBinary returns the binary encoding of the extended attributes.
func (a ExtendedAttributes) MarshalBinary() ([]byte, error) {
	buf := NewBuffer(make([]byte, 0, a.MarshalSize()))
	a.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom unmarshals the extended attributes from the buffer.
func (a *ExtendedAttributes) UnmarshalFrom(buf *Buffer) (err error) {
	count, err := buf.ConsumeCount()
	if err != nil {
		return err
	}

	*a = make([]ExtendedAttribute, count)
	for i := range count {
		(*a)[i].UnmarshalFrom(buf)
	}

	return buf.Err
}

// UnmarshalBinary decodes the binary encoding of the extended attributes.
func (a ExtendedAttributes) UnmarshalBinary(data []byte) error {
	return a.UnmarshalFrom(NewBuffer(data))
}

// Len returns the length of the extended attributes slice.
func (a ExtendedAttributes) Len() int {
	return len(a)
}

// AsMap converts the extended attributes into a map[string]string,
// where the type field is the key, and the data field is the value.
func (a ExtendedAttributes) AsMap() map[string]string {
	if len(a) == 0 {
		return nil
	}

	m := make(map[string]string, len(a))
	for _, ext := range a {
		m[ext.Type] = ext.Data
	}
	return m
}

// SetFromMap sets the extended attributes to an equivalent of the given map,
// where the key becomes the type field, and the value becomes the data field.
// For efficency the order is unspecified.
func (a *ExtendedAttributes) SetFromMap(m map[string]string) {
	*a = make(ExtendedAttributes, 0, len(m))
	for k, v := range m {
		*a = append(*a, ExtendedAttribute{
			Type: k,
			Data: v,
		})
	}
}

// Get returns the data field of the extended attribute where the key matches the type field.
//
// Since this method operates in linear time,
// if you are regularly accessing specific extended attributes,
// you may want to use [AsMap] instead.
func (a ExtendedAttributes) Get(key string) (data string, ok bool) {
	for _, ext := range a {
		if key == ext.Type {
			return ext.Data, true
		}
	}

	return "", false
}

// Seq is an iterator that yields the type field from each extended attribute.
func (a ExtendedAttributes) Seq(yield func(string) bool) {
	for _, ext := range a {
		if !yield(ext.Type) {
			return
		}
	}
}

// Seq2 is an iterator that yields the type and data fields from each extended attribute.
func (a ExtendedAttributes) Seq2(yield func(string, string) bool) {
	for _, ext := range a {
		if !yield(ext.Type, ext.Data) {
			return
		}
	}
}

// ExtendedAttribute defines the extended file attribute type defined in draft-ietf-secsh-filexfer-02
//
// Defined in: https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt#section-5
type ExtendedAttribute struct {
	Type string
	Data string
}

// MarshalSize returns the number of bytes the extended attribute would marshal into.
func (e *ExtendedAttribute) MarshalSize() int {
	// string(type) + string(data)
	return 4 + len(e.Type) + 4 + len(e.Data)
}

// MarshalInto marshals the extended attribute onto the end of the buffer.
func (e *ExtendedAttribute) MarshalInto(buf *Buffer) {
	buf.AppendString(e.Type)
	buf.AppendString(e.Data)
}

// MarshalBinary returns the binary encoding of the extended attribute.
func (e *ExtendedAttribute) MarshalBinary() ([]byte, error) {
	buf := NewBuffer(make([]byte, 0, e.MarshalSize()))
	e.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom unmarshals the extended attribute from the buffer.
func (e *ExtendedAttribute) UnmarshalFrom(buf *Buffer) (err error) {
	*e = ExtendedAttribute{
		Type: buf.ConsumeString(),
		Data: buf.ConsumeString(),
	}

	return buf.Err
}

// UnmarshalBinary decodes the binary encoding of the extended attribute.
func (e *ExtendedAttribute) UnmarshalBinary(data []byte) error {
	return e.UnmarshalFrom(NewBuffer(data))
}

// NameEntry implements the SSH_FXP_NAME repeated data type from draft-ietf-secsh-filexfer-02
//
// It implements both [fs.FileInfo] and [fs.DirEntry].
//
// This type is incompatible with versions 4 or higher.
type NameEntry struct {
	Filename string
	Longname string
	Attrs    Attributes
}

// Name implements [fs.FileInfo].
func (e *NameEntry) Name() string {
	return path.Base(e.Filename)
}

// Size implements [fs.FileInfo].
func (e *NameEntry) Size() int64 {
	return int64(e.Attrs.Size)
}

// Mode implements [fs.FileInfo].
func (e *NameEntry) Mode() fs.FileMode {
	return ToGoFileMode(e.Attrs.Permissions)
}

// ModTime implements [fs.FileInfo].
func (e *NameEntry) ModTime() time.Time {
	return time.Unix(int64(e.Attrs.MTime), 0)
}

// IsDir implements [fs.FileInfo].
func (e *NameEntry) IsDir() bool {
	return e.Attrs.Permissions.IsDir()
}

// Sys implements [fs.FileInfo].
// It returns a pointer of type *Attribute to the Attr field of this name entry.
func (e *NameEntry) Sys() any {
	return &e.Attrs
}

// Type implements [fs.DirEntry].
func (e *NameEntry) Type() fs.FileMode {
	return ToGoFileMode(e.Attrs.Permissions).Type()
}

// Info implements [fs.DirEntry].
func (e *NameEntry) Info() (fs.FileInfo, error) {
	return e, nil
}

// MarshalSize returns the number of bytes the name entry would marshal into.
func (e *NameEntry) MarshalSize() int {
	// string(filename) + string(longname) + ATTRS(attrs)
	return 4 + len(e.Filename) + 4 + len(e.Longname) + e.Attrs.MarshalSize()
}

// MarshalInto marshals the name entry onto the end of the buffer.
func (e *NameEntry) MarshalInto(buf *Buffer) {
	buf.AppendString(e.Filename)
	buf.AppendString(e.Longname)

	e.Attrs.MarshalInto(buf)
}

// MarshalBinary returns the binary encoding of the name entry.
func (e *NameEntry) MarshalBinary() ([]byte, error) {
	buf := NewBuffer(make([]byte, 0, e.MarshalSize()))
	e.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom unmarshals the name entry from the buffer.
func (e *NameEntry) UnmarshalFrom(buf *Buffer) (err error) {
	*e = NameEntry{
		Filename: buf.ConsumeString(),
		Longname: buf.ConsumeString(),
	}

	return e.Attrs.UnmarshalFrom(buf)
}

// UnmarshalBinary decodes the binary encoding of the name entry.
func (e *NameEntry) UnmarshalBinary(data []byte) error {
	return e.UnmarshalFrom(NewBuffer(data))
}
