package worker

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Validate actual directory bounds and records before archive/zip allocates.
// Footer counts alone are attacker-controlled and are only allocation hints.
func zipDirectoryEntries(data []byte) (int, error) {
	tailStart := max(0, len(data)-(22+65535))
	end := bytes.LastIndex(data[tailStart:], []byte("PK\x05\x06"))
	if end < 0 {
		return 0, fmt.Errorf("invalid ZIP directory")
	}
	end += tailStart
	footer := data[end:]
	if len(footer) < 22 {
		return 0, fmt.Errorf("truncated ZIP directory")
	}
	entries := int(binary.LittleEndian.Uint16(footer[10:12]))
	size := uint64(binary.LittleEndian.Uint32(footer[12:16]))
	offset := uint64(binary.LittleEndian.Uint32(footer[16:20]))
	if binary.LittleEndian.Uint16(footer[4:6]) != 0 || binary.LittleEndian.Uint16(footer[6:8]) != 0 ||
		binary.LittleEndian.Uint16(footer[8:10]) != uint16(entries) || entries > archiveMaxEntries || size > 1<<20 ||
		offset+size != uint64(end) || int(binary.LittleEndian.Uint16(footer[20:22])) != len(footer)-22 ||
		(end >= 20 && bytes.Equal(data[end-20:end-16], []byte("PK\x06\x07"))) {
		return 0, fmt.Errorf("ZIP directory exceeds limits or uses unsupported ZIP64/multiple disks")
	}
	directory := data[int(offset):end]
	count := 0
	for len(directory) > 0 {
		if len(directory) < 46 || !bytes.Equal(directory[:4], []byte("PK\x01\x02")) {
			return 0, fmt.Errorf("invalid ZIP directory record")
		}
		length := 46 + int(binary.LittleEndian.Uint16(directory[28:30])) + int(binary.LittleEndian.Uint16(directory[30:32])) + int(binary.LittleEndian.Uint16(directory[32:34]))
		if length > len(directory) {
			return 0, fmt.Errorf("truncated ZIP directory record")
		}
		count++
		if count > archiveMaxEntries || count > entries {
			return 0, fmt.Errorf("entry count exceeds limit or directory mismatch")
		}
		directory = directory[length:]
	}
	if count != entries {
		return 0, fmt.Errorf("entry count does not match ZIP directory")
	}
	return entries, nil
}
