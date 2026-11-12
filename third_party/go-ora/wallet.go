package go_ora

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/sha1"
	_ "crypto/sha1"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// type CertificateData
type wallet struct {
	file                string
	password            []byte
	salt                []byte
	sha1Iteration       int
	algType             int
	credentials         []walletCredential
	certificates        [][]byte
	privateKeys         [][]byte
	certificateRequests [][]byte
}
type walletCredential struct {
	dsn      string
	username string
	password string
}

// newWallet create new wallet object from file path
func newWallet(filePath string) (*wallet, error) {
	ret := new(wallet)
	ret.file = filePath
	err := ret.read()
	return ret, err
}

// read will read the file data decrypting file chunk to get wallet information
func (w *wallet) read() error {
	fileData, err := os.ReadFile(w.file)
	if err != nil {
		return err
	}
	index := 0
	if !bytes.Equal(fileData[index:index+3], []byte{161, 248, 78}) {
		return errors.New("TCPS: Invalid SSL Wallet (Magic)")
	}
	index += 3
	autoLoginLocal := false
	switch fileData[index] {
	case 54:
		fallthrough
	case 55:
		index += 1
	case 56:
		autoLoginLocal = true
		index += 1
	default:
		return errors.New("invalid magic version")
	}
	num1 := binary.BigEndian.Uint32(fileData[index : index+4])
	index += 4
	size := binary.BigEndian.Uint32(fileData[index : index+4])
	index += 4
	if num1 != 6 {
		return errors.New("invalid wallet header version")
	}
	num3 := fileData[index]
	if num3 == 5 {

	} else if num3 == 6 {
		index++
		rgbKey := fileData[index : index+16]
		index += 16
		blk, err := aes.NewCipher(rgbKey)
		if err != nil {
			return err
		}
		dec := cipher.NewCBCDecrypter(blk, []byte{192, 52, 216, 49, 28, 2, 206, 248, 81, 240, 20, 75, 129, 237, 75, 242})
		passwordLen := int(size) - 1 - 16
		w.password = make([]byte, passwordLen)
		dec.CryptBlocks(w.password, fileData[index:index+passwordLen])
		index += passwordLen
		if autoLoginLocal {
			hostname, _ := os.Hostname()
			currentUser := getCurrentUser()
			if idx := strings.Index(hostname, "."); idx != -1 {
				hostname = hostname[:idx]
			}
			key := []byte(hostname + currentUser.Name)
			mac := hmac.New(sha1.New, key)
			mac.Write(w.password)
			tempPassword := mac.Sum(nil)
			for x := 0; x < len(tempPassword); x++ {
				tempPassword[x] = (tempPassword[x]+128)%128%127 + 1
			}
			w.password = tempPassword[:16]
		}
	} else if num3 == 0x35 {
		index++
		rgbKey, err := hex.DecodeString(string(fileData[index : index+16]))
		if err != nil {
			return err
		}
		index += 16

		blk, err := des.NewCipher(rgbKey)
		if err != nil {
			return err
		}
		dec := cipher.NewCBCDecrypter(blk, []byte{0, 0, 0, 0, 0, 0, 0, 0})
		temp, err := hex.DecodeString(string(fileData[index : index+0x30]))
		if err != nil {
			return err
		}
		index += 0x30
		output := make([]byte, len(temp))
		dec.CryptBlocks(output, temp)
		num := int(output[len(output)-1])
		cutLen := 0
		if num <= dec.BlockSize() {
			apply := true
			for x := len(output) - num; x < len(output); x++ {
				if output[x] != uint8(num) {
					apply = false
					break
				}
			}
			if apply {
				cutLen = int(output[len(output)-1])
			}
			w.password = output[:len(output)-cutLen]
		} else {
			w.password = output
		}
	} else {
		return errors.New("invalid wallet header")
	}
	err = w.readPKCS12(fileData[index:])
	if err != nil {
		if autoLoginLocal {
			return fmt.Errorf("can't read wallet with auto login local properties: %v", err)
		}
	}
	return err
}

func (w *wallet) readPKCS12(data []byte) error {
	data, err := w.decodeASN1(data)
	if err != nil {
		return err
	}
	return w.readCredentials(data)
}

// readCredentials read dsn, usernames and passwords into walletCredentials array
func (w *wallet) readCredentials(input []byte) error {
	w.certificates = nil
	w.credentials = nil
	if input[1] == 130 {
		num2 := int(input[2])*256 + int(input[3])
		if len(input) < num2+4 {
			num3 := num2 + 4 - len(input)
			input = append(input, make([]byte, num3)...)
		}
	}
	type struct1 struct {
		Id   asn1.ObjectIdentifier
		Data asn1.RawValue
	}
	type WalletCredentialData struct {
		Id    string
		Value string
	}
	var (
		temp1 []struct1
		temp2 struct1
		temp3 WalletCredentialData
	)
	//objectType := 0
	_, err := asn1.Unmarshal(input, &temp1)
	if err != nil {
		return err
	}
	//var a []asn1.RawValue
	for _, tmp := range temp1 {
		// check the ContentType of the tmp first
		switch tmp.Id.String() {
		case "1.2.840.113549.1.12.10.1.5":
			_, err = asn1.Unmarshal(tmp.Data.Bytes, &temp2)
			if err != nil {
				return err
			}
			if temp2.Id.String() == "0.22.72.134.247.13.1.10" {
				// certificate request
				var a []byte
				_, err := asn1.Unmarshal(temp2.Data.Bytes, &a)
				if err != nil {
					return err
				}
				w.certificateRequests = append(w.certificateRequests, a)
			}
			if temp2.Id.String() != "1.2.840.113549.1.16.12.12" {
				continue
			}

			_, err = asn1.Unmarshal(temp2.Data.Bytes, &temp3)
			if err != nil {
				return err
			}
			r, err := regexp.Compile("(^.+)([0-9]+)")
			if err != nil {
				return err
			}
			matches := r.FindStringSubmatch(temp3.Id)
			if len(matches) != 3 {
				continue
			}
			length, err := strconv.Atoi(matches[2])
			if err != nil {
				continue
			}
			for len(w.credentials) < length {
				w.credentials = append(w.credentials, walletCredential{})
			}
			switch matches[1] {
			case "oracle.security.client.connect_string":
				w.credentials[length-1].dsn = temp3.Value
			case "oracle.security.client.username":
				w.credentials[length-1].username = temp3.Value
			case "oracle.security.client.password":
				w.credentials[length-1].password = temp3.Value
			default:
				return errors.New(fmt.Sprintf("cannot find entry for: %s", matches[1]))
			}
		case "1.2.840.113549.1.12.10.1.1":
			var a struct {
				Num int
				F1  struct {
					Id asn1.ObjectIdentifier
					F1 asn1.RawValue
				}
				PrivateKeyData []byte
			}
			_, err = asn1.Unmarshal(tmp.Data.Bytes, &a)
			if err != nil {
				return err
			}
			w.privateKeys = append(w.privateKeys, a.PrivateKeyData)
		case "1.2.840.113549.1.12.10.1.3":
			var a struct {
				Id asn1.ObjectIdentifier
				F1 struct {
					Data []byte
				} `asn1:"class:2,tag:0"`
			}
			_, err = asn1.Unmarshal(tmp.Data.Bytes, &a)
			if err != nil {
				return err
			}
			found := false
			for _, cert := range w.certificates {
				if bytes.Equal(cert, a.F1.Data) {
					found = true
					break
				}
			}
			if !found {
				w.certificates = append(w.certificates, a.F1.Data)
			}
		default:
			continue
		}

	}
	return nil
}

func (w *wallet) decodeASN1(buffer []byte) (data []byte, err error) {
	type contentInfo struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
	}
	type AlgorithmIdentifier struct {
		Algorithm  asn1.ObjectIdentifier
		Parameters asn1.RawValue `asn1:"optional"`
	}
	type pfxPdu struct {
		Version  int
		AuthSafe contentInfo
		MacData  struct {
			Mac struct {
				Algorithm AlgorithmIdentifier
				Digest    []byte
			}
			MacSalt    []byte
			Iterations int `asn1:"optional,default:1"`
		} `asn1:"optional"`
	}
	type encryptedContentInfo struct {
		ContentType                asn1.ObjectIdentifier
		ContentEncryptionAlgorithm AlgorithmIdentifier
		EncryptedContent           []byte `asn1:"tag:0,optional"`
	}
	type encryptedData struct {
		Version              int
		EncryptedContentInfo encryptedContentInfo
	}
	type pbeParams struct {
		Salt       []byte
		Iterations int
	}

	type pbes2Params struct {
		Kdf              AlgorithmIdentifier
		EncryptionScheme AlgorithmIdentifier
	}
	type pbkdf2Params struct {
		Salt       asn1.RawValue
		Iterations int
		KeyLength  int                 `asn1:"optional"`
		Prf        AlgorithmIdentifier `asn1:"optional"`
	}
	var (
		pfx               pfxPdu
		authenticatedSafe []contentInfo
		temp              encryptedData
	)
	_, err = asn1.Unmarshal(buffer, &pfx)
	if err != nil {
		return
	}
	if pfx.Version < 2 {
		err = errors.New("error in reading wallet")
		return
	}
	if !pfx.AuthSafe.ContentType.Equal(oidDataContentType) {
		err = errors.New(fmt.Sprintf("error in reading wallet: invalid object ID received: %s, want: %s",
			pfx.AuthSafe.ContentType.String(), "1.2.840.113549.1.7.1"))
		return
	}
	_, err = asn1.Unmarshal(pfx.AuthSafe.Content.Bytes, &pfx.AuthSafe.Content)
	if err != nil {
		return
	}
	_, err = asn1.Unmarshal(pfx.AuthSafe.Content.Bytes, &authenticatedSafe)
	if err != nil {
		return
	}
	var index = -1
	for idx, obj := range authenticatedSafe {
		if obj.ContentType.Equal(oidEncryptedDataContentType) {
			index = idx
			break
		}
	}
	if index == -1 {
		err = errors.New(fmt.Sprintf("error in reading wallet: object ID: %s is not present",
			"1.2.840.113549.1.7.6"))
		return
	}
	_, err = asn1.Unmarshal(authenticatedSafe[index].Content.Bytes, &temp)
	if err != nil {
		return
	}
	if !temp.EncryptedContentInfo.ContentType.Equal(oidDataContentType) {
		err = errors.New(fmt.Sprintf("error in reading wallet: invalid object ID received: %s, want: %s",
			temp.EncryptedContentInfo.ContentType.String(), "1.2.840.113549.1.7.1"))
		return
	}
	algorithm := temp.EncryptedContentInfo.ContentEncryptionAlgorithm.Algorithm.String()
	var algo walletAlgorithm
	switch algorithm {
	case "1.2.840.113549.1.12.1.6":
		err = errors.New("RC2 wallet decryption is not supported")
		return
	case "1.2.840.113549.1.12.1.3":
		var params pbeParams
		_, err = asn1.Unmarshal(temp.EncryptedContentInfo.ContentEncryptionAlgorithm.Parameters.FullBytes, &params)
		if err != nil {
			return
		}
		algo = &shaWithTripleDESCBC{
			defaultAlgorithm{
				password:  w.password,
				salt:      params.Salt,
				iteration: params.Iterations,
			},
		}
	case "1.2.840.113549.1.5.13":
		var params pbes2Params
		if _, err = asn1.Unmarshal(temp.EncryptedContentInfo.ContentEncryptionAlgorithm.Parameters.FullBytes, &params); err != nil {
			return
		}

		if !params.Kdf.Algorithm.Equal(oidPBKDF2) {
			err = errors.New("kdf algorithm " + params.Kdf.Algorithm.String() + " is not supported")
			return
		}
		var kdfParams pbkdf2Params
		if _, err = asn1.Unmarshal(params.Kdf.Parameters.FullBytes, &kdfParams); err != nil {
			return
		}
		if kdfParams.Salt.Tag != asn1.TagOctetString {
			err = errors.New("pkcs12: only octet string salts are supported for pbkdf2")
			return
		}
		//var prf hash.Hash
		var h func() hash.Hash
		var keyLen int
		// get hash type
		switch {
		case kdfParams.Prf.Algorithm.Equal(oidHmacWithSHA256):
			h = sha256.New
		case kdfParams.Prf.Algorithm.Equal(oidHmacWithSHA1):
			h = sha1.New
		case kdfParams.Prf.Algorithm.Equal([]int{}):
			h = sha1.New
		default:
			err = errors.New("pbes2 prf " + kdfParams.Prf.Algorithm.String() + " is not supported")
			return
		}

		// get key length
		switch {
		case params.EncryptionScheme.Algorithm.Equal(oidAES256CBC):
			keyLen = 32
		case params.EncryptionScheme.Algorithm.Equal(oidAES192CBC):
			keyLen = 24
		case params.EncryptionScheme.Algorithm.Equal(oidAES128CBC):
			keyLen = 16
		default:
			err = errors.New("pbes2 algorithm " + params.EncryptionScheme.Algorithm.String() + " is not supported")
			return
		}
		algo = &pbkdf2{
			defaultAlgorithm: defaultAlgorithm{
				password:  w.password,
				salt:      kdfParams.Salt.Bytes,
				iv:        params.EncryptionScheme.Parameters.Bytes,
				iteration: kdfParams.Iterations,
			},
			hash:   h,
			keyLen: keyLen,
		}
	default:
		err = fmt.Errorf("algorithm %s is not supported", algorithm)
		return
	}
	err = algo.create()
	if err != nil {
		return
	}
	data, err = decrypt(algo, temp.EncryptedContentInfo.EncryptedContent)
	return
}

// getCredential read one credential dsn, username, password from encrypted data
func (w *wallet) getCredential(server string, port int, service, username string) (*walletCredential, error) {
	rHost, err := regexp.Compile(`\(\s*HOST\s*=\s*([A-z0-9._%+-]+)\)`)
	if err != nil {
		return nil, err
	}
	rPort, err := regexp.Compile(`\(\s*PORT\s*=\s*([0-9]+)\)`)
	if err != nil {
		return nil, err
	}
	rService, err := regexp.Compile(`\(\s*SERVICE_NAME\s*=\s*([A-Z0-9._%+-]+)\)`)
	if err != nil {
		return nil, err
	}
	var (
		lhost    string
		lport    int
		lservice string
	)
	for _, cred := range w.credentials {
		if username != "" {
			if strings.ToUpper(username) != strings.ToUpper(cred.username) {
				continue
			}
		}
		matches := rHost.FindStringSubmatch(strings.ToUpper(cred.dsn))
		if len(matches) != 2 {
			continue
		}
		lhost = strings.TrimSpace(matches[1])
		matches = rPort.FindStringSubmatch(strings.ToUpper(cred.dsn))
		if len(matches) == 2 {
			lport, err = strconv.Atoi(matches[1])
			if err != nil {
				lport = defaultPort
			}
		} else {
			lport = defaultPort
		}
		matches = rService.FindStringSubmatch(strings.ToUpper(cred.dsn))
		if len(matches) != 2 {
			continue
		}
		lservice = strings.TrimSpace(matches[1])
		if port == 0 {
			port = 1521
		}
		if lhost == strings.ToUpper(server) &&
			lport == port &&
			lservice == strings.ToUpper(service) {
			return &cred, nil
		}
	}
	return nil, nil
}
