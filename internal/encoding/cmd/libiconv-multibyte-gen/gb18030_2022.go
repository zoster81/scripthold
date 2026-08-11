package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type gb18030DecodePatch struct {
	bytes  []byte
	r      uint32
	reject bool
}

type gb18030EncodePatch struct {
	r      uint32
	bytes  []byte
	reject bool
}

type gb18030PatchSet struct {
	headerSHA256 string
	decode       []gb18030DecodePatch
	encode       []gb18030EncodePatch
}

func probeGB180302022(source, work, gcc string) (gb18030PatchSet, error) {
	headerPath := filepath.Join(source, "lib", "gb18030_2022.h")
	headerBytes, err := os.ReadFile(headerPath)
	if err != nil {
		return gb18030PatchSet{}, err
	}
	cPath := filepath.Join(work, "gb18030-2022-diff.c")
	executable := filepath.Join(work, "gb18030-2022-diff")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	if err := os.WriteFile(cPath, []byte(gb180302022ProbeSource), 0o600); err != nil {
		return gb18030PatchSet{}, err
	}
	compile := exec.Command(gcc, "-std=c99", "-O2", "-I", filepath.Join(source, "lib"), cPath, "-o", executable)
	if output, err := compile.CombinedOutput(); err != nil {
		return gb18030PatchSet{}, fmt.Errorf("compile failed: %w: %s", err, string(output))
	}
	cmd := exec.Command(executable)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return gb18030PatchSet{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return gb18030PatchSet{}, err
	}
	reader := bufio.NewReaderSize(stdout, 256*1024)
	magic := make([]byte, 5)
	if _, err := io.ReadFull(reader, magic); err != nil {
		return gb18030PatchSet{}, err
	}
	if !bytes.Equal(magic, []byte{'G', 'B', '2', '2', 1}) {
		return gb18030PatchSet{}, fmt.Errorf("unexpected GB18030:2022 probe magic %x", magic)
	}

	base := simplifiedchinese.GB18030
	baseDecoder := base.NewDecoder()
	baseEncoder := base.NewEncoder()
	baseReplacement, err := base.NewEncoder().Bytes([]byte("\ufffd"))
	if err != nil || len(baseReplacement) == 0 {
		return gb18030PatchSet{}, fmt.Errorf("x/text GB18030 cannot encode U+FFFD: %v", err)
	}
	patches := gb18030PatchSet{headerSHA256: sha256Hex(headerBytes)}
	section, err := reader.ReadByte()
	if err != nil || section != 'D' {
		return gb18030PatchSet{}, fmt.Errorf("GB18030:2022 missing decode section: section=%q err=%v", section, err)
	}
	decodeCount, err := readProbeRune(reader)
	if err != nil {
		return gb18030PatchSet{}, fmt.Errorf("read decode record count: %w", err)
	}
	for index := uint32(0); index < decodeCount; index++ {
		length, err := reader.ReadByte()
		if err != nil || length != 2 && length != 4 {
			return gb18030PatchSet{}, fmt.Errorf("decode record %d invalid length %d: %v", index, length, err)
		}
		var sequenceBuffer [4]byte
		if _, err := io.ReadFull(reader, sequenceBuffer[:]); err != nil {
			return gb18030PatchSet{}, fmt.Errorf("decode record %d bytes: %w", index, err)
		}
		pinned, err := readProbeRune(reader)
		if err != nil {
			return gb18030PatchSet{}, fmt.Errorf("decode record %d rune: %w", index, err)
		}
		sequence := sequenceBuffer[:length]
		baseRune, baseValid := decodeBaseGB18030(baseDecoder, baseReplacement, sequence)
		appendDecodePatchIfDifferent(&patches.decode, sequence, pinned, baseRune, baseValid)
	}

	section, err = reader.ReadByte()
	if err != nil || section != 'E' {
		return gb18030PatchSet{}, fmt.Errorf("GB18030:2022 missing encode section: section=%q err=%v", section, err)
	}
	encodeCount, err := readProbeRune(reader)
	if err != nil {
		return gb18030PatchSet{}, fmt.Errorf("read encode record count: %w", err)
	}
	var runeBytes [4]byte
	var encoded [8]byte
	var previousRune uint32
	for index := uint32(0); index < encodeCount; index++ {
		pinnedRune, err := readProbeRune(reader)
		if err != nil {
			return gb18030PatchSet{}, fmt.Errorf("encode record %d rune: %w", index, err)
		}
		if pinnedRune > utf8.MaxRune || pinnedRune >= 0xd800 && pinnedRune <= 0xdfff || index > 0 && pinnedRune <= previousRune {
			return gb18030PatchSet{}, fmt.Errorf("encode record %d invalid/out-of-order rune U+%X", index, pinnedRune)
		}
		previousRune = pinnedRune
		pinnedLength, err := reader.ReadByte()
		if err != nil {
			return gb18030PatchSet{}, fmt.Errorf("encode record U+%04X length: %w", pinnedRune, err)
		}
		if _, err := io.ReadFull(reader, runeBytes[:]); err != nil {
			return gb18030PatchSet{}, fmt.Errorf("encode record U+%04X bytes: %w", pinnedRune, err)
		}
		var pinned []byte
		if pinnedLength > 0 {
			if pinnedLength > 4 {
				return gb18030PatchSet{}, fmt.Errorf("invalid pinned encoder length %d for U+%04X", pinnedLength, pinnedRune)
			}
			pinned = append([]byte(nil), runeBytes[:pinnedLength]...)
		}
		r := rune(pinnedRune)
		baseEncoder.Reset()
		var source [4]byte
		sourceLength := utf8.EncodeRune(source[:], r)
		nDst, nSrc, baseErr := baseEncoder.Transform(encoded[:], source[:sourceLength], true)
		baseValid := baseErr == nil && nSrc == sourceLength
		var baseBytes []byte
		if baseValid {
			baseBytes = encoded[:nDst]
		}
		if pinnedLength == 0 {
			if baseValid {
				patches.encode = append(patches.encode, gb18030EncodePatch{r: pinnedRune, reject: true})
			}
			continue
		}
		if !baseValid || !bytes.Equal(baseBytes, pinned) {
			patches.encode = append(patches.encode, gb18030EncodePatch{r: pinnedRune, bytes: pinned})
		}
	}
	if extra, err := reader.ReadByte(); err != io.EOF {
		if err == nil {
			return gb18030PatchSet{}, fmt.Errorf("GB18030:2022 probe emitted trailing byte 0x%02X", extra)
		}
		return gb18030PatchSet{}, fmt.Errorf("read GB18030:2022 probe trailer: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return gb18030PatchSet{}, fmt.Errorf("GB18030:2022 probe failed: %w: %s", err, stderr.String())
	}
	return patches, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}

func readProbeRune(reader *bufio.Reader) (uint32, error) {
	var data [4]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return 0, err
	}
	return uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]), nil
}

func decodeBaseGB18030(decoder *encoding.Decoder, replacement []byte, sequence []byte) (uint32, bool) {
	decoder.Reset()
	var dst [8]byte
	nDst, nSrc, err := decoder.Transform(dst[:], sequence, true)
	if err != nil || nSrc != len(sequence) || !utf8.Valid(dst[:nDst]) {
		return 0, false
	}
	r, size := utf8.DecodeRune(dst[:nDst])
	if size != nDst {
		return 0, false
	}
	if r == utf8.RuneError && !bytes.Equal(sequence, replacement) {
		return 0, false
	}
	return uint32(r), true
}

func appendDecodePatchIfDifferent(target *[]gb18030DecodePatch, sequence []byte, pinned, base uint32, baseValid bool) {
	const invalid = uint32(0xffffffff)
	if pinned == invalid {
		if baseValid {
			*target = append(*target, gb18030DecodePatch{bytes: append([]byte(nil), sequence...), reject: true})
		}
		return
	}
	if !baseValid || pinned != base {
		*target = append(*target, gb18030DecodePatch{bytes: append([]byte(nil), sequence...), r: pinned})
	}
}

const gb180302022ProbeSource = `
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#endif
typedef unsigned int ucs4_t; typedef unsigned int state_t;
struct conv_struct { unsigned int isurface; state_t istate; unsigned int osurface; state_t ostate; }; typedef struct conv_struct * conv_t;
#define RET_SHIFT_ILSEQ(n) (-1-2*(n))
#define RET_ILSEQ RET_SHIFT_ILSEQ(0)
#define RET_TOOFEW(n) (-2-2*(n))
#define RET_ILUNI -1
#define RET_TOOSMALL -2
#define ICONV_SURFACE_EBCDIC_ZOS_UNIX 1U
static unsigned char swap_x15_x25(unsigned char x){return (unsigned char)(x ^ ((((x)-0x15)&~0x10)==0 ? 0x30 : 0));}
typedef struct { unsigned short indx; unsigned short used; } Summary16;
#include "ascii.h"
#include "gb2312.h"
#include "gbk.h"
#include "gb18030ext.h"
#include "gb18030uni.h"
#include "gb18030_2022.h"
static void put_u32(uint32_t v){fputc((v>>24)&255,stdout);fputc((v>>16)&255,stdout);fputc((v>>8)&255,stdout);fputc(v&255,stdout);}
static void decode_record(const unsigned char *in,size_t n){unsigned char padded[4]={0};memcpy(padded,in,n);struct conv_struct c={0};ucs4_t wc=0;int r=gb18030_2022_mbtowc(&c,&wc,in,n);fputc((int)n,stdout);fwrite(padded,1,4,stdout);if(r==(int)n)put_u32(wc);else put_u32(0xffffffffU);}
int main(void){
#ifdef _WIN32
 _setmode(_fileno(stdout), _O_BINARY);
#endif
 const unsigned char magic[5]={'G','B','2','2',1};fwrite(magic,1,5,stdout);
 fputc('D',stdout);put_u32(1611540U);
 for(unsigned a=0x81;a<=0xfe;a++)for(unsigned b=0x40;b<=0xfe;b++){if(b==0x7f)continue;unsigned char in[2]={(unsigned char)a,(unsigned char)b};decode_record(in,2);}
 for(unsigned a=0x81;a<=0xfe;a++)for(unsigned b=0x30;b<=0x39;b++)for(unsigned c=0x81;c<=0xfe;c++)for(unsigned d=0x30;d<=0x39;d++){unsigned char in[4]={(unsigned char)a,(unsigned char)b,(unsigned char)c,(unsigned char)d};decode_record(in,4);}
 fputc('E',stdout);put_u32(1112064U);
 for(ucs4_t wc=0;wc<=0x10ffff;wc++){if(wc>=0xd800&&wc<=0xdfff)continue;struct conv_struct c={0};unsigned char out[4]={0};int r=gb18030_2022_wctomb(&c,out,wc,sizeof(out));put_u32(wc);if(r<0){fputc(0,stdout);fwrite(out,1,4,stdout);}else{fputc(r,stdout);fwrite(out,1,4,stdout);}}
 return ferror(stdout)?2:0;
}
`
