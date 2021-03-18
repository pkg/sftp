package filexfer

// ExtensionPair defines the extension-pair type defined in draft-ietf-secsh-filexfer-13.
// This type is backwards-compatible with how draft-ietf-secsh-filexfer-02 defines extensions.
//
// Defined in: https://tools.ietf.org/html/draft-ietf-secsh-filexfer-13#section-4.2
type ExtensionPair struct {
	Name string
	Data string
}

// MarshalInto marshals e onto the end of the given Buffer.
func (e *ExtensionPair) MarshalInto(buf *Buffer) int {
	buf.AppendString(e.Name)
	buf.AppendString(e.Data)

	return 4 + len(e.Name) + 4 + len(e.Data)
}

// UnmarshalFrom unmarshals an ExtensionPair from the given Buffer into e.
func (e *ExtensionPair) UnmarshalFrom(buf *Buffer) (err error) {
	if e.Name, err = buf.ConsumeString(); err != nil {
		return err
	}

	if e.Data, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}
