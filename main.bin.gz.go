// Copyright (c) 2021-2022 Nadeen Udantha <me@nadeen.lk>. All rights reserved.

package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

func main() {
	if len(os.Args) == 4 {
		if os.Args[1] == "e" {
			enc()
		} else if os.Args[1] == "d" {
			dec()
		}
	}
}

func fuck(err error) {
	if err != nil {
		panic(err)
	}
}

var (
	iww   = 640
	ihh   = 360
	scale = 4
	iw    = iww / scale
	ih    = ihh / scale
	siw   = iw * scale
	sih   = ih * scale
	buf   = make([]byte, (iw*ih)/8)
	img   = make([]byte, iw*ih)
)

func enc() {
	in, err := os.OpenFile(os.Args[2], os.O_RDONLY, 0777)
	fuck(err)
	pr, pw := io.Pipe()
	gz, err := gzip.NewWriterLevel(pw, gzip.BestSpeed)
	fuck(err)
	go func() {
		io.Copy(gz, in)
		in.Close()
		gz.Close()
		pw.Close()
		pr.Close()
	}()
	fstat, err := in.Stat()
	fuck(err)
	fsz := int(fstat.Size())
	defer in.Close()
	cmd := `ffmpeg -r 15 -f rawvideo -s %dx%d -pix_fmt gray -i -`
	cmd += ` -vf scale=%d:%d:flags=neighbor+bitexact,format=yuv420p`
	cmd += ` -force_key_frames expr:gte(t,n_forced/2) -bf 2`
	cmd += ` -r 30 -crf 18 -c:v libx264 -b:v 1K -y %s`
	ffmpeg := exec.Command("cmd", "/c", fmt.Sprintf(cmd, iw, ih, siw, sih, os.Args[3]))
	func() {
		r, err := ffmpeg.StderrPipe()
		fuck(err)
		go io.Copy(os.Stderr, r)
		r, err = ffmpeg.StdoutPipe()
		fuck(err)
		go io.Copy(os.Stdout, r)
	}()
	w, err := ffmpeg.StdinPipe()
	fuck(err)
	fuck(ffmpeg.Start())
	defer ffmpeg.Process.Kill()
	go func() {
		defer w.Close()
		bps := 0
		tbs := 0
		go func() {
			for {
				fmt.Printf("\n%d %d/%d\n", bps, tbs, fsz)
				bps = 0
				time.Sleep(time.Second)
			}
		}()
		for {
			n, err := io.ReadAtLeast(pr, buf, len(buf))
			if n == 0 {
				break
			}
			if n != len(buf) {
				for x := n; x < len(buf); x++ {
					buf[x] = 0
				}
			}
			bps += len(buf)
			tbs += len(buf)
			for x := 0; x < len(buf); x++ {
				for y := 0; y < 8; y++ {
					img[x*8+y] = ((buf[x] >> y) & 1) * 255
				}
			}
			_, err2 := w.Write(img)
			fuck(err2)
			if err != nil {
				break
			}
		}
	}()
	fuck(ffmpeg.Wait())
}

func dec() {
	out, err := os.OpenFile(os.Args[3], os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0777)
	fuck(err)
	defer out.Close()
	pr, pw := io.Pipe()
	go func() {
		gz, err := gzip.NewReader(pr)
		fuck(err)
		io.Copy(out, gz)
		gz.Close()
		out.Close()
		pw.Close()
		pr.Close()
	}()
	ffmpeg := exec.Command("cmd", "/c", fmt.Sprintf(`ffmpeg -i %s -f rawvideo -vf scale=%d:%d:flags=bitexact -pix_fmt gray -`, os.Args[2], iw, ih))
	func() {
		r, err := ffmpeg.StderrPipe()
		fuck(err)
		go io.Copy(os.Stderr, r)
	}()
	r, err := ffmpeg.StdoutPipe()
	fuck(err)
	fuck(ffmpeg.Start())
	defer ffmpeg.Process.Kill()
	oldbuf := make([]byte, len(buf))
	for {
		_, err = io.ReadAtLeast(r, img, len(img))
		if err != nil {
			break
		}
		for z := 0; z < len(buf); z++ {
			buf[z] = 0
		}
		for x := 0; x < len(buf); x++ {
			for y := 0; y < 8; y++ {
				v := img[x*8+y]
				if v >= 128 {
					v = 1
				} else {
					v = 0
				}
				buf[x] |= v << y
			}
		}
		if bytes.Equal(buf, oldbuf) {
			fmt.Println("dup")
			continue
		}
		copy(oldbuf, buf)
		_, err := pw.Write(buf)
		if errors.Is(err, io.ErrClosedPipe) {
			break
		}
		fuck(err)
	}
}
