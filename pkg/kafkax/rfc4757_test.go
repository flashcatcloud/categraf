package kafkax

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rc4"
	"encoding/hex"
	"strconv"
	"testing"

	krbcrypto "github.com/jcmturner/gokrb5/v8/crypto"
	"github.com/jcmturner/gokrb5/v8/crypto/rfc4757"
)

func TestRC4HMACKeyUsageEncoding(t *testing.T) {
	for _, tc := range []struct {
		usage uint32
		want  string
	}{
		{0, "00000000"},
		{1, "01000000"},
		{3, "08000000"},
		{8, "08000000"},
		{9, "08000000"},
		{23, "0d000000"},
		{127, "7f000000"},
		{128, "80000000"},
		{255, "ff000000"},
		{256, "00010000"},
		{1024, "00040000"},
		{0x0fffffff, "ffffff0f"},
		{0x10000000, "00000010"},
		{0xffffffff, "ffffffff"},
	} {
		t.Run(strconv.FormatUint(uint64(tc.usage), 10), func(t *testing.T) {
			if got := rfc4757.UsageToMSMsgType(tc.usage); len(got) != 4 || hex.EncodeToString(got) != tc.want {
				t.Fatalf("usage=%d encoded=%x want=%s", tc.usage, got, tc.want)
			}
		})
	}
}

func TestRC4HMACApplicationKeyUsage(t *testing.T) {
	key := decodeKerberosHex(t, "00112233445566778899aabbccddeeff")
	payload := []byte("categraf Kerberos application payload")
	// Fixed wire vectors generated independently using RFC 4757 sections 3-5,
	// Python hashlib/hmac and RC4, with confounder 0102030405060708. A round trip
	// through the same implementation would miss a shared key-encoding defect.
	for _, tc := range []struct {
		usage      uint32
		encoded    string
		checksum   string
		ciphertext string
	}{
		{3, "08000000", "0faab92c19914961213efd06206ebf8c", "0b7324b80a1cee25ccceb1da37029397f458262e51ca8baba07c4aca5a33ad0dacd86ef891804cd77f7dd11b459cc0784b6ac174cf6109a032550f0c9f"},
		{9, "08000000", "0faab92c19914961213efd06206ebf8c", "0b7324b80a1cee25ccceb1da37029397f458262e51ca8baba07c4aca5a33ad0dacd86ef891804cd77f7dd11b459cc0784b6ac174cf6109a032550f0c9f"},
		{23, "0d000000", "efb7ddab06951cc238c81bff301beb0b", "c44e7a7308c1a2dfdcab3225419c7aa1edeed03d0c409bb42767c9e3c542527c4396dd499455e1ad6a9f35efcb4e16b68c30cc25a5c4fcaaa279f130d4"},
		{1024, "00040000", "7a97e105f2e5ddc054225a17e1629d3c", "9896cdc45bf9011eb671feefd7b13af0b99a640485ae29c85a4c17efab0f336338626ec7d32ea67d9b53f918c1329eb045952c691791567ca98af5158e"},
		{0x10000000, "00000010", "f2f2cd1fc855f2284de53a16ed042f1b", "8e0220fe4a19e495cb8b49ce97e4da650c4e5cbbb8c3a9feb1970e7cdfd2f04b961c06f41ffb1f793845fc6cf5ef3573641b4c7cf03af07d44ea34724f"},
		{0xffffffff, "ffffffff", "2844f76cd263025daaa2412e9a2a177e", "75b39471ab5a743a33708b29593ad18022e1f5b8a5bf944ddcb2a1eb7bf6f58a4471f33e691752355e96146ec784b868fa3b8086f1e109451f800e4dc5"},
	} {
		t.Run(strconv.FormatUint(uint64(tc.usage), 10), func(t *testing.T) {
			etype := krbcrypto.RC4HMAC{}
			t.Run("checksum", func(t *testing.T) {
				got, err := etype.GetChecksumHash(key, payload, tc.usage)
				if err != nil || hex.EncodeToString(got) != tc.checksum {
					t.Fatalf("checksum=%x err=%v want=%s", got, err, tc.checksum)
				}
				if !etype.VerifyChecksum(key, payload, decodeKerberosHex(t, tc.checksum), tc.usage) {
					t.Fatal("rejected independent checksum")
				}
			})
			t.Run("decrypt", func(t *testing.T) {
				got, err := etype.DecryptMessage(key, decodeKerberosHex(t, tc.ciphertext), tc.usage)
				if err != nil || !bytes.Equal(got, payload) {
					t.Fatalf("decrypted=%x err=%v", got, err)
				}
			})
			t.Run("encrypt", func(t *testing.T) {
				_, ciphertext, err := etype.EncryptMessage(key, payload, tc.usage)
				if err != nil {
					t.Fatal(err)
				}
				if len(ciphertext) != md5.Size+8+len(payload) {
					t.Fatalf("ciphertext length=%d", len(ciphertext))
				}
				// Verify the wire output with standard-library primitives and the
				// fixed usage bytes, independently of gokrb5's encoding/decryption.
				k2 := kerberosHMACMD5(key, decodeKerberosHex(t, tc.encoded))
				tag := ciphertext[:md5.Size]
				stream, err := rc4.NewCipher(kerberosHMACMD5(k2, tag))
				if err != nil {
					t.Fatal(err)
				}
				plain := make([]byte, len(ciphertext)-md5.Size)
				stream.XORKeyStream(plain, ciphertext[md5.Size:])
				if !bytes.Equal(plain[8:], payload) || !hmac.Equal(tag, kerberosHMACMD5(k2, plain)) {
					t.Fatal("ciphertext violates RFC 4757 wire encoding")
				}
			})
		})
	}
}

func decodeKerberosHex(t *testing.T, encoded string) []byte {
	t.Helper()
	b, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func kerberosHMACMD5(key, data []byte) []byte {
	mac := hmac.New(md5.New, key)
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}
