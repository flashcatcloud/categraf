package go_ora

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"encoding/binary"
	"hash"
)

type walletAlgorithm interface {
	create() error
	getIV() []byte
	getBlock() cipher.Block
}
type defaultAlgorithm struct {
	password  []byte
	salt      []byte
	iv        []byte
	iteration int
	blk       cipher.Block
}
type shaWithTripleDESCBC struct {
	defaultAlgorithm
}

type pbkdf2 struct {
	defaultAlgorithm
	hash func() hash.Hash

	keyLen int
}

func (algo *pbkdf2) create() error {
	//origPass, _ := decodeBMPString(convertToBigEndianUtf16(algo.password))
	//origPass := convertToBigEndianUtf16(algo.password)
	prf := hmac.New(algo.hash, algo.password)
	hashLen := prf.Size()
	numBlocks := (algo.keyLen + hashLen - 1) / hashLen
	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	U := make([]byte, hashLen)
	for block := 1; block <= numBlocks; block++ {
		// N.B.: || means concatenation, ^ means XOR
		// for each block T_i = U_1 ^ U_2 ^ ... ^ U_iter
		// U_1 = PRF(password, salt || uint(i))
		prf.Reset()
		prf.Write(algo.salt)
		binary.BigEndian.PutUint32(buf[:4], uint32(block))
		//buf[0] = byte(block >> 24)
		//buf[1] = byte(block >> 16)
		//buf[2] = byte(block >> 8)
		//buf[3] = byte(block)
		prf.Write(buf[:4])
		dk = prf.Sum(dk)
		T := dk[len(dk)-hashLen:]
		copy(U, T)

		// U_n = PRF(password, U_(n-1))
		for n := 2; n <= algo.iteration; n++ {
			prf.Reset()
			prf.Write(U)
			U = U[:0]
			U = prf.Sum(U)
			for x := range U {
				T[x] ^= U[x]
			}
		}
	}
	var err error
	algo.blk, err = aes.NewCipher(dk[:algo.keyLen])
	if err != nil {
		return err
	}
	return nil
}

func (algo *shaWithTripleDESCBC) create() error {
	to2 := fillWithRepeats(convertToBigEndianUtf16(algo.password), 64)
	to3 := fillWithRepeats(algo.salt, 64)
	hashKey1 := produceHash(bytes.Repeat([]byte{1}, 64), to3, to2, algo.iteration)
	algo.iv = append(make([]byte, 0), produceHash(bytes.Repeat([]byte{2}, 64), to3, to2, algo.iteration)[:8]...)
	to1 := fillWithRepeats(hashKey1, 64)
	num3 := 1
	num4 := 1
	num5 := 64
	for num5 > 0 {
		num5--
		num6 := num3 + int(to2[num5]) + int(to1[num5])
		to2[num5] = uint8(num6)
		num3 = num6 >> 8
		num7 := num4 + int(to3[num5]) + int(to1[num5])
		to3[num5] = uint8(num7)
		num4 = num7 >> 8
	}
	hashKey2 := produceHash(bytes.Repeat([]byte{1}, 64), to3, to2, algo.iteration)
	key := append(hashKey1, hashKey2[:4]...)
	var err error
	algo.blk, err = des.NewTripleDESCipher(key)
	if err != nil {
		return err
	}
	return nil
}

func (algo *defaultAlgorithm) getIV() []byte {
	return algo.iv
}
func (algo *defaultAlgorithm) getBlock() cipher.Block {
	return algo.blk
}
