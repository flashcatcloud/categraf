package go_ora

import (
	"bytes"
	"crypto"
	"crypto/cipher"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

var (
	oidDataContentType          = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 7, 1})
	oidEncryptedDataContentType = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 7, 6})

	oidFriendlyName     = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 9, 20})
	oidLocalKeyID       = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 9, 21})
	oidMicrosoftCSPName = asn1.ObjectIdentifier([]int{1, 3, 6, 1, 4, 1, 311, 17, 1})

	oidPBEWithSHAAnd3KeyTripleDESCBC = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 1, 3})
	oidPBEWithSHAAnd128BitRC2CBC     = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 1, 5})
	oidPBEWithSHAAnd40BitRC2CBC      = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 1, 6})
	oidPBES2                         = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 5, 13})
	oidPBKDF2                        = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 5, 12})
	oidHmacWithSHA1                  = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 2, 7})
	oidHmacWithSHA256                = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 2, 9})
	oidAES128CBC                     = asn1.ObjectIdentifier([]int{2, 16, 840, 1, 101, 3, 4, 1, 2})
	oidAES192CBC                     = asn1.ObjectIdentifier([]int{2, 16, 840, 1, 101, 3, 4, 1, 22})
	oidAES256CBC                     = asn1.ObjectIdentifier([]int{2, 16, 840, 1, 101, 3, 4, 1, 42})
)

func fillWithRepeats(input []byte, v int) []byte {
	output := append(make([]byte, 0, v), input...)
	size := v - (len(input) % v)
	if size != v {
		for size >= len(input) {
			output = append(output, input...)
			size -= len(input)
		}
	}
	if size > 0 {
		output = append(output, input[:size]...)
	}
	return output
}

func convertToBigEndianUtf16(input []byte) []byte {
	temp := utf16.Encode([]rune(string(input)))
	output := make([]byte, 0, (len(temp)*2)+2)
	for x := 0; x < len(temp); x++ {
		tempByte := []byte{0, 0}
		binary.BigEndian.PutUint16(tempByte, temp[x])
		output = append(output, tempByte...)
	}
	output = append(output, 0, 0)
	return output
}

func produceHash(buff1, buff2, buff3 []byte, iter int) []byte {
	hash := crypto.SHA1.New()
	hash.Write(buff1)
	hash.Write(buff2)
	hash.Write(buff3)
	result := hash.Sum(nil)
	for x := 0; x < iter-1; x++ {
		hash.Reset()
		hash.Write(result)
		result = hash.Sum(nil)
	}
	return result
}

func decrypt(algo walletAlgorithm, input []byte) ([]byte, error) {
	blk := algo.getBlock()
	iv := algo.getIV()
	cbc := cipher.NewCBCDecrypter(blk, iv)
	output := make([]byte, len(input))
	cbc.CryptBlocks(output, input)
	// remove padding
	if int(output[len(output)-1]) <= blk.BlockSize() {
		num := int(output[len(output)-1])
		padding := bytes.Repeat([]byte{uint8(num)}, num)
		if bytes.Equal(output[len(output)-num:], padding) {
			output = output[:len(output)-num]
		}
	}
	return output, nil
}

func decodeBMPString(bmpString []byte) (string, error) {
	if len(bmpString)%2 != 0 {
		return "", errors.New("pkcs12: odd-length BMP string")
	}

	// strip terminator if present
	if l := len(bmpString); l >= 2 && bmpString[l-1] == 0 && bmpString[l-2] == 0 {
		bmpString = bmpString[:l-2]
	}

	s := make([]uint16, 0, len(bmpString)/2)
	for len(bmpString) > 0 {
		s = append(s, uint16(bmpString[0])<<8+uint16(bmpString[1]))
		bmpString = bmpString[2:]
	}

	return string(utf16.Decode(s)), nil
}
