package verthazel

import (
	"fmt"
	"io"
	"os"
)

type RWHandle func(p []byte) (int, error)

type IORW struct {
	rawbuffer      [][]byte
	rawbytes       []byte
	rawbytesi      int
	rwBufSize      int
	altw           io.Writer
	rbufferi       int
	rbytes         []byte
	rbytesi        int
	singleB        []byte
	altWriteHandle RWHandle
}

func (ioRW *IORW) CleanupIORW() {
	if ioRW.rawbuffer != nil {
		for len(ioRW.rawbuffer) > 0 {
			ioRW.rawbuffer[0] = nil
			if len(ioRW.rawbuffer) > 1 {
				ioRW.rawbuffer = ioRW.rawbuffer[1:]
			} else {
				break
			}
		}
		ioRW.rawbuffer = nil
	}
	if ioRW.rawbytes != nil {
		ioRW.rawbytes = nil
	}
	if ioRW.rawbytesi > 0 {
		ioRW.rawbytesi = 0
	}
	if ioRW.singleB != nil {
		ioRW.singleB = nil
	}
}

func (ioRW *IORW) InputFileFromPath(fpath string) (err error) {
	if f, ferr := os.Open(fpath); ferr == nil {
		err = ioRW.InputFile(f)
	} else {
		err = ferr
	}
	return err
}

func (ioRW *IORW) RWBufferSize() int {
	if ioRW.rwBufSize <= 0 {
		ioRW.rwBufSize = 4096
	}
	return ioRW.rwBufSize
}

func (ioRW *IORW) InputFile(f *os.File) (err error) {
	if f != nil {
		err = ioRW.InputReader(f)
	}
	return err
}

var emptyIORW *IORW

func EmptyIO() *IORW {
	if emptyIORW == nil {
		emptyIORW = &IORW{}
	}
	return emptyIORW
}

func (ioRW *IORW) InputReader(r io.Reader) (err error) {
	if rw, ok := r.(*IORW); ok {
		rw.ReadToWriter(ioRW)
		return
	}
	if r != nil {
		p := make([]byte, ioRW.RWBufferSize())
		for {
			nr, nrerr := r.Read(p)
			if nr > 0 {
				ni := 0
				for ni < nr {
					nw, nwerr := ioRW.Write(p[ni : ni+(nr-ni)])
					if nw > 0 {
						ni += nw
					} else {
						if nwerr != nil {
							nrerr = nwerr
						}
						break
					}
					if nwerr != nil {
						nrerr = nwerr
						break
					}
				}
			}
			if nrerr != nil {
				err = nrerr
				break
			}
		}
	}
	return err
}

func (ioRW *IORW) Print(a ...interface{}) {
	for _, bi := range a {
		if bufrw, isbufrw := bi.(*IORW); isbufrw {
			if len(bufrw.rawbuffer) > 0 {
				for _, buf := range bufrw.rawbuffer {
					ioRW.Write(buf)
				}
			}
			if bufrw.rawbytesi > 0 {
				ioRW.Write(bufrw.rawbytes[0:bufrw.rawbytesi])
			}
		} else if reader, isReader := bi.(io.Reader); isReader {
			ioRW.InputReader(reader)
		} else if bs, isBytes := bi.([]byte); isBytes {
			if len(bs) > 0 {
				ioRW.Write(bs)
			}
		} else if s, isString := bi.(string); isString {
			ioRW.Write([]byte(s))
		} else {
			fmt.Fprint(ioRW, bi)
		}
	}
}

func (ioRW *IORW) Println(a ...interface{}) {
	ioRW.Print(a...)
	fmt.Fprintln(ioRW)
}

func (ioRW *IORW) Empty() bool {
	if ioRW.rawbuffer == nil || len(ioRW.rawbuffer) == 0 {
		return ioRW.rawbytesi == 0
	}
	return false
}

func (ioRW *IORW) WriteFromReader(r io.Reader) (err error) {
	if r != nil {
		wbuffer := make([]byte, ioRW.RWBufferSize())
		wbufferi := int(0)
		wbufferl := int(0)
		for err == nil {
			wbufferl, err = r.Read(wbuffer)
			for wbufferl > 0 {
				if wbufferi < wbufferl {
					wl, werr := ioRW.Write(wbuffer[wbufferi : wbufferi+(wbufferl-wbufferi)])
					if wl == 0 {
						if werr == nil {
							werr = io.EOF
						}
					} else {
						wbufferi += wl
					}
					if werr != nil {
						err = werr
					}
				} else {
					break
				}
			}
			wbufferi = 0
		}
	}
	return err
}

func (ioRW *IORW) ReadToWriter(w io.Writer) (err error) {
	if w != nil {
		if ioRW.Empty() {
			w.Write(EmptyBytes())
		} else {
			for {
				nib := int(0)

				if len(ioRW.rawbuffer) > 0 {
					for _, ib := range ioRW.rawbuffer {
						for nib < len(ib) {
							hn, herr := w.Write(ib[nib : nib+(len(ib)-nib)])
							nib += hn
							if herr != nil {
								err = herr
								break
							}
						}

						if err != nil {
							break
						}
						nib = 0
					}
					if err != nil {
						break
					}
				}

				if ioRW.rawbytesi > 0 {
					for nib < ioRW.rawbytesi {
						hn, herr := w.Write(ioRW.rawbytes[nib : nib+(ioRW.rawbytesi-nib)])
						nib += hn
						if herr != nil {
							err = herr
							break
						}
					}
				}
				break
			}
		}
	}
	return err
}

func (ioRW *IORW) SubString(index uint64, length uint64) (s string) {
	if !ioRW.Empty() {
		ioRW.ReadToHandler(func(p []byte) (n int, err error) {
			if n = len(p); n > 0 {
				for _, b := range p {
					if index > 0 {
						index--
					} else {
						if length > 0 {
							length--
							s += string(b)
							if length == 0 {
								err = io.EOF
								break
							}
						} else {
							err = io.EOF
							break
						}
					}
				}
			}
			return n, err
		})
	}
	return s
}

func (ioRW *IORW) StartsWith(s string) (startWith bool) {
	if !ioRW.Empty() {
		startWith = ioRW.startWithEndWith(true, []byte(s))
	}
	return startWith
}

func (ioRW *IORW) Length() (l uint64) {
	if !ioRW.Empty() {
		l += (uint64(len(ioRW.rawbuffer)) * uint64(ioRW.RWBufferSize()))
		l += uint64(ioRW.rawbytesi)
	}
	return l
}

func (ioRW *IORW) startWithEndWith(testStart bool, p []byte) bool {
	if pl := len(p); pl > 0 && !ioRW.Empty() {
		doneTestingStartEnd := make(chan bool)
		defer close(doneTestingStartEnd)
		go func() {
			pi := 0
			if testStart {
				if len(ioRW.rawbuffer) > 0 {
					for _, rawbuf := range ioRW.rawbuffer {
						for _, b := range rawbuf {
							if p[pi] == b {
								pi++
								if pi == pl {
									doneTestingStartEnd <- true
									return
								}
							} else {
								doneTestingStartEnd <- false
								return
							}
						}
					}
				}
				if ioRW.rawbytesi > 0 {
					for _, b := range ioRW.rawbytes[0:ioRW.rawbytesi] {
						if p[pi] == b {
							pi++
							if pi == pl {
								doneTestingStartEnd <- true
								return
							}
						} else {
							doneTestingStartEnd <- false
							return
						}
					}
				}

			} else {
				if ioRW.rawbytesi > 0 {
					testbuf := ioRW.rawbytes[0:ioRW.rawbytesi]
					for n, _ := range testbuf {
						if pi < pl {
							if p[pl-(pi+1)] == testbuf[len(testbuf)-(n+1)] {
								pi++
								if pi == pl {
									doneTestingStartEnd <- true
									return
								}
							} else {
								doneTestingStartEnd <- false
								return
							}
						} else {
							doneTestingStartEnd <- false
							return
						}
					}
				}
				if pi < pl {
					if len(ioRW.rawbuffer) > 0 {
						for nbuf, _ := range ioRW.rawbuffer {
							rawbuf := ioRW.rawbuffer[len(ioRW.rawbuffer)-(nbuf+1)]
							for n, _ := range rawbuf {
								if pi < pl {
									if p[pl-(pi+1)] == rawbuf[len(rawbuf)-(n+1)] {
										pi++
										if pi == pl {
											doneTestingStartEnd <- true
											return
										}
									} else {
										doneTestingStartEnd <- false
										return
									}
								} else {
									doneTestingStartEnd <- false
									return
								}
							}
						}
					}
				}
			}
			doneTestingStartEnd <- false
		}()
		return <-doneTestingStartEnd
	}
	return false
}

func (ioRW *IORW) EndsWith(s string) (endWith bool) {
	if !ioRW.Empty() {
		endWith = ioRW.startWithEndWith(false, []byte(s))
	}
	return endWith
}

func (ioRW *IORW) String() (s string) {
	if !ioRW.Empty() {
		if len(ioRW.rawbuffer) > 0 {
			for _, iob := range ioRW.rawbuffer {
				s += string(iob)
			}
		}
		if ioRW.rawbytesi > 0 {
			s += string(ioRW.rawbytes[0:ioRW.rawbytesi])
		}
	}
	return s
}

func (ioRW *IORW) Read(p []byte) (n int, err error) {
	if pl := len(p); pl > 0 {
		if ioRW.Empty() {
			err = io.EOF
		} else {
			for n < pl {
				if len(ioRW.rbytes) == 0 {
					if len(ioRW.rawbuffer) > 0 && ioRW.rbufferi < len(ioRW.rawbuffer) {
						ioRW.rbytes = ioRW.rawbuffer[ioRW.rbufferi]
						ioRW.rbytesi = 0
					} else if ioRW.rawbytesi > 0 && ioRW.rbytesi < ioRW.rawbytesi {
						ioRW.rbytes = ioRW.rawbytes[0:ioRW.rawbytesi]
					} else {
						err = io.EOF
						break
					}
				}
				if (pl - n) >= (len(ioRW.rbytes) - ioRW.rbytesi) {
					c := copy(p[n:n+(len(ioRW.rbytes)-ioRW.rbytesi)], ioRW.rbytes[ioRW.rbytesi:ioRW.rbytesi+(len(ioRW.rbytes)-ioRW.rbytesi)])
					ioRW.rbytesi += c
					n += c
				} else if (pl - n) < (len(ioRW.rbytes) - ioRW.rbytesi) {
					c := copy(p[n:n+(pl-n)], ioRW.rbytes[ioRW.rbytesi:ioRW.rbytesi+(pl-n)])
					ioRW.rbytesi += c
					n += c
				}
				if len(ioRW.rbytes) == ioRW.rbytesi {
					if ioRW.rbufferi < len(ioRW.rawbuffer) {
						ioRW.rbytes = nil
						ioRW.rbytesi = 0
						ioRW.rbufferi++
						if ioRW.rbufferi == len(ioRW.rawbuffer) {
							if ioRW.rbytesi < ioRW.rawbytesi {
								ioRW.rbytes = ioRW.rawbytes[ioRW.rbytesi:ioRW.rawbytesi]
							}
						}
					} else if ioRW.rbytesi == ioRW.rawbytesi {
						err = io.EOF
						break
					}
				}
			}
		}
	}
	return n, err
}

func (ioRW *IORW) WriteByte(b byte) (n int, err error) {
	if ioRW.singleB == nil {
		ioRW.singleB = make([]byte, 1)
	}
	ioRW.singleB[0] = b
	n, err = ioRW.Write(ioRW.singleB)
	return n, err
}

func (ioRW *IORW) Write(p []byte) (n int, err error) {
	if pl := len(p); pl > 0 {
		if ioRW.altWriteHandle != nil {
			n, err = RWHandle(ioRW.altWriteHandle)(p)
		} else {
			c := 0
			for n < pl {
				if len(ioRW.rawbytes) == 0 || ioRW.rawbytes == nil {
					ioRW.rawbytes = make([]byte, ioRW.RWBufferSize())
					ioRW.rawbytesi = 0
				}
				if (pl - n) >= (len(ioRW.rawbytes) - ioRW.rawbytesi) {
					if c = copy(ioRW.rawbytes[ioRW.rawbytesi:ioRW.rawbytesi+(len(ioRW.rawbytes)-ioRW.rawbytesi)], p[n:n+(len(ioRW.rawbytes)-ioRW.rawbytesi)]); c > 0 {
						n += c
						ioRW.rawbytesi += c
					}
				} else if (pl - n) < (len(ioRW.rawbytes) - ioRW.rawbytesi) {
					if c = copy(ioRW.rawbytes[ioRW.rawbytesi:ioRW.rawbytesi+(pl-n)], p[n:n+(pl-n)]); c > 0 {
						n += c
						ioRW.rawbytesi += c
					}
				}
				if len(ioRW.rawbytes) == ioRW.rawbytesi {
					if ioRW.rawbuffer == nil {
						ioRW.rawbuffer = [][]byte{}
					}
					ioRW.rawbuffer = append(ioRW.rawbuffer, ioRW.rawbytes[:])
					ioRW.rawbytes = nil
					ioRW.rawbytesi = 0
				}
				if c == 0 {
					break
				}
			}
		}
	}
	return n, err
}

func (ioRW *IORW) ReadToHandler(handler RWHandle) (err error) {
	if handler != nil {
		if ioRW.Empty() {
			handler(emptyBytes)
		} else {
			for {
				nib := int(0)

				if len(ioRW.rawbuffer) > 0 {
					for _, ib := range ioRW.rawbuffer {
						for nib < len(ib) {
							hn, herr := handler(ib[nib : nib+(len(ib)-nib)])
							nib += hn
							if herr != nil {
								err = herr
								break
							}
						}

						if err != nil {
							break
						}
						nib = 0
					}
					if err != nil {
						break
					}
				}

				if ioRW.rawbytesi > 0 {
					for nib < ioRW.rawbytesi {
						hn, herr := handler(ioRW.rawbytes[nib : nib+(ioRW.rawbytesi-nib)])
						nib += hn
						if herr != nil {
							err = herr
							break
						}
					}
				}
				break
			}
		}
	}
	return err
}

func (ioRW *IORW) NewIORWCursor() (ioIORWCursor *IORWCursor) {
	ioIORWCursor = &IORWCursor{ioRW: ioRW}
	return ioIORWCursor
}

type IORWCursor struct {
	ioRW      *IORW
	totalRead uint64
}

var emptyBytes []byte

func EmptyBytes() []byte {
	if emptyBytes == nil {
		emptyBytes = []byte{}
	}
	return emptyBytes
}

func init() {
	if emptyBytes == nil {
		emptyBytes = []byte{}
	}
}

type ReturnVal struct {
	n int
	e error
}
