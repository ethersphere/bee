package mock

import "bytes"

type StreamMock struct {
	readBuffer        *bytes.Buffer
	writeBuffer       *bytes.Buffer
	writeCounter      int
	readCounter       int
	readError         error
	writeError        error
	readErrCheckmark  int
	writeErrCheckmark int
}

func NewStream(readBuffer, writeBuffer *bytes.Buffer) *StreamMock {
	return &StreamMock{readBuffer: readBuffer, writeBuffer: writeBuffer}
}
func (s *StreamMock) SetReadErr(err error, checkmark int) {
	s.readError = err
	s.readErrCheckmark = checkmark

}

func (s *StreamMock) SetWriteErr(err error, checkmark int) {
	s.writeError = err
	s.writeErrCheckmark = checkmark

}

func (s *StreamMock) Read(p []byte) (n int, err error) {
	if s.readError != nil && s.readErrCheckmark <= s.readCounter {
		return 0, s.readError
	}

	s.readCounter++
	return s.readBuffer.Read(p)
}

func (s *StreamMock) Write(p []byte) (n int, err error) {
	if s.writeError != nil && s.writeErrCheckmark <= s.writeCounter {
		return 0, s.writeError
	}

	s.writeCounter++
	return s.writeBuffer.Write(p)
}

func (s *StreamMock) Close() error {
	return nil
}
