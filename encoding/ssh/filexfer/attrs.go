package filexfer

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
// Defined in: https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-5
type Attributes struct {
	Flags uint32

	// AttrSize
	Size uint64

	// AttrUIDGID
	UID uint32
	GID uint32

	// AttrPermissions
	Permissions uint32

	// AttrACmodTime
	ATime uint32
	MTime uint32

	// AttrExtended
	ExtendedAttributes []ExtendedAttribute
}

// Len returns the number of bytes a would marshal into.
func (a *Attributes) Len() int {
	length := 4

	if a.Flags&AttrSize != 0 {
		length += 8
	}

	if a.Flags&AttrUIDGID != 0 {
		length += 4 + 4
	}

	if a.Flags&AttrPermissions != 0 {
		length += 4
	}

	if a.Flags&AttrACModTime != 0 {
		length += 4 + 4
	}

	if a.Flags&AttrExtended != 0 {
		length += 4

		for _, ext := range a.ExtendedAttributes {
			length += ext.Len()
		}
	}

	return length
}

// MarshalInto marshals e onto the end of the given Buffer.
func (a *Attributes) MarshalInto(b *Buffer) {
	b.AppendUint32(a.Flags)

	if a.Flags&AttrSize != 0 {
		b.AppendUint64(a.Size)
	}

	if a.Flags&AttrUIDGID != 0 {
		b.AppendUint32(a.UID)
		b.AppendUint32(a.GID)
	}

	if a.Flags&AttrPermissions != 0 {
		b.AppendUint32(a.Permissions)
	}

	if a.Flags&AttrACModTime != 0 {
		b.AppendUint32(a.ATime)
		b.AppendUint32(a.MTime)
	}

	if a.Flags&AttrExtended != 0 {
		b.AppendUint32(uint32(len(a.ExtendedAttributes)))

		for _, ext := range a.ExtendedAttributes {
			ext.MarshalInto(b)
		}
	}
}

// MarshalBinary returns a as the binary encoding of a.
func (a *Attributes) MarshalBinary() ([]byte, error) {
	buf := NewBuffer(make([]byte, a.Len()))
	a.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom unmarshals an Attributes from the given Buffer into e.
//
// NOTE: The values of fields not covered in the a.Flags are explicitly undefined.
func (a *Attributes) UnmarshalFrom(b *Buffer) (err error) {
	if a.Flags, err = b.ConsumeUint32(); err != nil {
		return err
	}

	// Short-circuit dummy attributes.
	if a.Flags == 0 {
		return nil
	}

	if a.Flags&AttrSize != 0 {
		if a.Size, err = b.ConsumeUint64(); err != nil {
			return err
		}
	}

	if a.Flags&AttrUIDGID != 0 {
		if a.UID, err = b.ConsumeUint32(); err != nil {
			return err
		}

		if a.GID, err = b.ConsumeUint32(); err != nil {
			return err
		}
	}

	if a.Flags&AttrPermissions != 0 {
		if a.Permissions, err = b.ConsumeUint32(); err != nil {
			return err
		}
	}

	if a.Flags&AttrACModTime != 0 {
		if a.ATime, err = b.ConsumeUint32(); err != nil {
			return err
		}

		if a.MTime, err = b.ConsumeUint32(); err != nil {
			return err
		}
	}

	if a.Flags&AttrExtended != 0 {
		count, err := b.ConsumeUint32()
		if err != nil {
			return err
		}

		a.ExtendedAttributes = make([]ExtendedAttribute, count)
		for i := range a.ExtendedAttributes {
			a.ExtendedAttributes[i].UnmarshalFrom(b)
		}
	}

	return nil
}

// UnmarshalBinary decodes the binary encoding of Attributes into e.
func (a *Attributes) UnmarshalBinary(data []byte) error {
	return a.UnmarshalFrom(NewBuffer(data))
}

// ExtendedAttribute defines the extended file attribute type defined in draft-ietf-secsh-filexfer-02
//
// Defined in: https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-5
type ExtendedAttribute struct {
	Type string
	Data string
}

// Len returns the number of bytes e would marshal into.
func (e *ExtendedAttribute) Len() int {
	return 4 + len(e.Type) + 4 + len(e.Data)
}

// MarshalInto marshals e onto the end of the given Buffer.
func (e *ExtendedAttribute) MarshalInto(b *Buffer) {
	b.AppendString(e.Type)
	b.AppendString(e.Data)
}

// MarshalBinary returns e as the binary encoding of e.
func (e *ExtendedAttribute) MarshalBinary() ([]byte, error) {
	buf := NewBuffer(make([]byte, e.Len()))
	e.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom unmarshals an ExtendedAattribute from the given Buffer into e.
func (e *ExtendedAttribute) UnmarshalFrom(b *Buffer) (err error) {
	if e.Type, err = b.ConsumeString(); err != nil {
		return err
	}

	if e.Data, err = b.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary decodes the binary encoding of ExtendedAttribute into e.
func (e *ExtendedAttribute) UnmarshalBinary(data []byte) error {
	return e.UnmarshalFrom(NewBuffer(data))
}

// NameEntry implements the SSH_FXP_NAME repeated data type from draft-ietf-secsh-filexfer-02
//
// This type is incompatible with versions 4 or higher.
type NameEntry struct {
	Filename string
	Longname string
	Attrs    Attributes
}

// Len returns the number of bytes e would marshal into.
func (e *NameEntry) Len() int {
	return 4 + len(e.Filename) + 4 + len(e.Longname) + e.Attrs.Len()
}

// MarshalInto marshals e onto the end of the given Buffer.
func (e *NameEntry) MarshalInto(b *Buffer) {
	b.AppendString(e.Filename)
	b.AppendString(e.Longname)

	e.Attrs.MarshalInto(b)
}

// MarshalBinary returns e as the binary encoding of e.
func (e *NameEntry) MarshalBinary() ([]byte, error) {
	buf := NewBuffer(make([]byte, e.Len()))
	e.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom unmarshals an NameEntry from the given Buffer into e.
//
// NOTE: The values of fields not covered in the a.Flags are explicitly undefined.
func (e *NameEntry) UnmarshalFrom(b *Buffer) (err error) {
	if e.Filename, err = b.ConsumeString(); err != nil {
		return err
	}

	if e.Longname, err = b.ConsumeString(); err != nil {
		return err
	}

	return e.Attrs.UnmarshalFrom(b)
}

// UnmarshalBinary decodes the binary encoding of NameEntry into e.
func (e *NameEntry) UnmarshalBinary(data []byte) error {
	return e.UnmarshalFrom(NewBuffer(data))
}
