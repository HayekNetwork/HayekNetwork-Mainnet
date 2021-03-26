package main

import (
    "fmt"
    "encoding/hex"
)

const HashLength = 32

type Hash [HashLength]byte

func BytesToHash(b []byte) Hash {
    var h Hash
    h.SetBytes(b)
    return h
}

func (h *Hash) SetBytes(b []byte) {
    if len(b) > len(h) {
        b = b[len(b)-HashLength:]
    }

    copy(h[HashLength-len(b):], b)
}

func Hex2Bytes(str string) []byte {
    h, _ := hex.DecodeString(str)

    return h
}

func FromHex(s string) []byte {
    if len(s) > 1 {
        if s[0:2] == "0x" || s[0:2] == "0X" {
            s = s[2:]
        }
    }
    if len(s)%2 == 1 {
        s = "0" + s
    }
    return Hex2Bytes(s)
}

func HexToHash(s string) Hash    { return BytesToHash(FromHex(s)) }

func main() {
    h := BytesToHash([]byte("0x11bbe8db4e347b4e8c937c1c8370e4b5ed33adb3db69cbdb7a38e1e50b1b82fa"));
    fmt.Println(h)
}
