package crypto

import (
	"bytes"
	"io/ioutil"
	"net/mail"
	"net/textproto"
	"strings"

	gomime "github.com/ProtonMail/go-mime"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/packet"
)

func (pgp GopenPGP) parseMIME(
	mimeBody string, verifierKey *KeyRing,
) (*gomime.BodyCollector, int, []string, []string, error) {
	mm, err := mail.ReadMessage(strings.NewReader(mimeBody))
	if err != nil {
		return nil, 0, nil, nil, err
	}
	config := &packet.Config{DefaultCipher: packet.CipherAES256, Time: pgp.getTimeGenerator()}

	h := textproto.MIMEHeader(mm.Header)
	mmBodyData, err := ioutil.ReadAll(mm.Body)
	if err != nil {
		return nil, 0, nil, nil, err
	}

	printAccepter := gomime.NewMIMEPrinter()
	bodyCollector := gomime.NewBodyCollector(printAccepter)
	attachmentsCollector := gomime.NewAttachmentsCollector(bodyCollector)
	mimeVisitor := gomime.NewMimeVisitor(attachmentsCollector)

	var pgpKering openpgp.KeyRing
	if verifierKey != nil {
		pgpKering = verifierKey.entities
	}

	signatureCollector := newSignatureCollector(mimeVisitor, pgpKering, config)

	err = gomime.VisitAll(bytes.NewReader(mmBodyData), h, signatureCollector)

	verified := signatureCollector.verified
	body := bodyCollector
	atts := attachmentsCollector.GetAttachments()
	attHeaders := attachmentsCollector.GetAttHeaders()

	return body, verified, atts, attHeaders, err
}

// MIMECallbacks defines callback methods to process a MIME message.
type MIMECallbacks interface {
	OnBody(body string, mimetype string)
	OnAttachment(headers string, data []byte)
	// Encrypted headers can be in an attachment and thus be placed at the end of the mime structure.
	OnEncryptedHeaders(headers string)
	OnVerified(verified int)
	OnError(err error)
}

// DecryptMIMEMessage decrypts a MIME message.
func (pgp *GopenPGP) DecryptMIMEMessage(
	encryptedText string, verifierKey, privateKeyRing *KeyRing,
	passphrase string, callbacks MIMECallbacks, verifyTime int64,
) {
	decsignverify, err := pgp.DecryptMessageVerify(encryptedText, verifierKey, privateKeyRing, passphrase, verifyTime)
	if err != nil {
		callbacks.OnError(err)
		return
	}

	body, verified, attachments, attachmentHeaders, err := pgp.parseMIME(decsignverify.Plaintext, verifierKey)
	if err != nil {
		callbacks.OnError(err)
		return
	}
	bodyContent, bodyMimeType := body.GetBody()
	callbacks.OnBody(bodyContent, bodyMimeType)
	for i := 0; i < len(attachments); i++ {
		callbacks.OnAttachment(attachmentHeaders[i], []byte(attachments[i]))
	}
	callbacks.OnEncryptedHeaders("")
	if decsignverify.Verify == notSigned {
		callbacks.OnVerified(verified)
	} else {
		callbacks.OnVerified(decsignverify.Verify)
	}
}
