package filexfer

// Attributes related flags.
const (
	AttrSize        = 1 << iota // SSH_FILEXFER_ATTR_SIZE
	AttrUIDGID                  // SSH_FILEXFER_ATTR_UIDGID
	AttrPermissions             // SSH_FILEXFER_ATTR_PERMISSIONS
	AttrACModTime               // SSH_FILEXFER_ACMODTIME

	AttrExtended = (1 << 31) // SSH_FILEXFER_ATTR_EXTENDED
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

// ExtendedAttribute defines the extended file attribute type defined in draft-ietf-secsh-filexfer-02
//
// Defined in: https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-5
type ExtendedAttribute struct {
	Type string
	Data string
}

// MarshalInto marshals e onto the end of the given Buffer.
func (e *ExtendedAttribute) MarshalInto(b *Buffer) {
	b.AppendString(e.Type)
	b.AppendString(e.Data)
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
