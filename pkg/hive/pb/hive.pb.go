// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: hive.proto

package pb

import (
	fmt "fmt"
	proto "github.com/gogo/protobuf/proto"
	io "io"
	math "math"
	math_bits "math/bits"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion3 // please upgrade the proto package

type Subscribe struct {
	Depth uint32 `protobuf:"varint,1,opt,name=Depth,proto3" json:"Depth,omitempty"`
}

func (m *Subscribe) Reset()         { *m = Subscribe{} }
func (m *Subscribe) String() string { return proto.CompactTextString(m) }
func (*Subscribe) ProtoMessage()    {}
func (*Subscribe) Descriptor() ([]byte, []int) {
	return fileDescriptor_d635d1ead41ba02c, []int{0}
}
func (m *Subscribe) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Subscribe) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Subscribe.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Subscribe) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Subscribe.Merge(m, src)
}
func (m *Subscribe) XXX_Size() int {
	return m.Size()
}
func (m *Subscribe) XXX_DiscardUnknown() {
	xxx_messageInfo_Subscribe.DiscardUnknown(m)
}

var xxx_messageInfo_Subscribe proto.InternalMessageInfo

func (m *Subscribe) GetDepth() uint32 {
	if m != nil {
		return m.Depth
	}
	return 0
}

type SubscribeResponse struct {
	Peers *Peers `protobuf:"bytes,1,opt,name=Peers,proto3" json:"Peers,omitempty"`
}

func (m *SubscribeResponse) Reset()         { *m = SubscribeResponse{} }
func (m *SubscribeResponse) String() string { return proto.CompactTextString(m) }
func (*SubscribeResponse) ProtoMessage()    {}
func (*SubscribeResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_d635d1ead41ba02c, []int{1}
}
func (m *SubscribeResponse) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *SubscribeResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_SubscribeResponse.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *SubscribeResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_SubscribeResponse.Merge(m, src)
}
func (m *SubscribeResponse) XXX_Size() int {
	return m.Size()
}
func (m *SubscribeResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_SubscribeResponse.DiscardUnknown(m)
}

var xxx_messageInfo_SubscribeResponse proto.InternalMessageInfo

func (m *SubscribeResponse) GetPeers() *Peers {
	if m != nil {
		return m.Peers
	}
	return nil
}

type Peers struct {
	BzzAddress []*BzzAddress `protobuf:"bytes,1,rep,name=BzzAddress,proto3" json:"BzzAddress,omitempty"`
}

func (m *Peers) Reset()         { *m = Peers{} }
func (m *Peers) String() string { return proto.CompactTextString(m) }
func (*Peers) ProtoMessage()    {}
func (*Peers) Descriptor() ([]byte, []int) {
	return fileDescriptor_d635d1ead41ba02c, []int{2}
}
func (m *Peers) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Peers) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Peers.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Peers) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Peers.Merge(m, src)
}
func (m *Peers) XXX_Size() int {
	return m.Size()
}
func (m *Peers) XXX_DiscardUnknown() {
	xxx_messageInfo_Peers.DiscardUnknown(m)
}

var xxx_messageInfo_Peers proto.InternalMessageInfo

func (m *Peers) GetBzzAddress() []*BzzAddress {
	if m != nil {
		return m.BzzAddress
	}
	return nil
}

type BzzAddress struct {
	Overlay  []byte `protobuf:"bytes,1,opt,name=Overlay,proto3" json:"Overlay,omitempty"`
	Underlay []byte `protobuf:"bytes,2,opt,name=Underlay,proto3" json:"Underlay,omitempty"`
}

func (m *BzzAddress) Reset()         { *m = BzzAddress{} }
func (m *BzzAddress) String() string { return proto.CompactTextString(m) }
func (*BzzAddress) ProtoMessage()    {}
func (*BzzAddress) Descriptor() ([]byte, []int) {
	return fileDescriptor_d635d1ead41ba02c, []int{3}
}
func (m *BzzAddress) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *BzzAddress) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_BzzAddress.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *BzzAddress) XXX_Merge(src proto.Message) {
	xxx_messageInfo_BzzAddress.Merge(m, src)
}
func (m *BzzAddress) XXX_Size() int {
	return m.Size()
}
func (m *BzzAddress) XXX_DiscardUnknown() {
	xxx_messageInfo_BzzAddress.DiscardUnknown(m)
}

var xxx_messageInfo_BzzAddress proto.InternalMessageInfo

func (m *BzzAddress) GetOverlay() []byte {
	if m != nil {
		return m.Overlay
	}
	return nil
}

func (m *BzzAddress) GetUnderlay() []byte {
	if m != nil {
		return m.Underlay
	}
	return nil
}

type Depth struct {
	Depth uint32 `protobuf:"varint,1,opt,name=Depth,proto3" json:"Depth,omitempty"`
}

func (m *Depth) Reset()         { *m = Depth{} }
func (m *Depth) String() string { return proto.CompactTextString(m) }
func (*Depth) ProtoMessage()    {}
func (*Depth) Descriptor() ([]byte, []int) {
	return fileDescriptor_d635d1ead41ba02c, []int{4}
}
func (m *Depth) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Depth) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Depth.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Depth) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Depth.Merge(m, src)
}
func (m *Depth) XXX_Size() int {
	return m.Size()
}
func (m *Depth) XXX_DiscardUnknown() {
	xxx_messageInfo_Depth.DiscardUnknown(m)
}

var xxx_messageInfo_Depth proto.InternalMessageInfo

func (m *Depth) GetDepth() uint32 {
	if m != nil {
		return m.Depth
	}
	return 0
}

func init() {
	proto.RegisterType((*Subscribe)(nil), "pb.Subscribe")
	proto.RegisterType((*SubscribeResponse)(nil), "pb.SubscribeResponse")
	proto.RegisterType((*Peers)(nil), "pb.Peers")
	proto.RegisterType((*BzzAddress)(nil), "pb.BzzAddress")
	proto.RegisterType((*Depth)(nil), "pb.Depth")
}

func init() { proto.RegisterFile("hive.proto", fileDescriptor_d635d1ead41ba02c) }

var fileDescriptor_d635d1ead41ba02c = []byte{
	// 213 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0xca, 0xc8, 0x2c, 0x4b,
	0xd5, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0x62, 0x2a, 0x48, 0x52, 0x52, 0xe4, 0xe2, 0x0c, 0x2e,
	0x4d, 0x2a, 0x4e, 0x2e, 0xca, 0x4c, 0x4a, 0x15, 0x12, 0xe1, 0x62, 0x75, 0x49, 0x2d, 0x28, 0xc9,
	0x90, 0x60, 0x54, 0x60, 0xd4, 0xe0, 0x0d, 0x82, 0x70, 0x94, 0x4c, 0xb8, 0x04, 0xe1, 0x4a, 0x82,
	0x52, 0x8b, 0x0b, 0xf2, 0xf3, 0x8a, 0x53, 0x85, 0xe4, 0xb9, 0x58, 0x03, 0x52, 0x53, 0x8b, 0x8a,
	0xc1, 0x4a, 0xb9, 0x8d, 0x38, 0xf5, 0x0a, 0x92, 0xf4, 0xc0, 0x02, 0x41, 0x10, 0x71, 0x25, 0x73,
	0xa8, 0x02, 0x21, 0x3d, 0x2e, 0x2e, 0xa7, 0xaa, 0x2a, 0xc7, 0x94, 0x94, 0xa2, 0xd4, 0x62, 0x90,
	0x72, 0x66, 0x0d, 0x6e, 0x23, 0x3e, 0x90, 0x72, 0x84, 0x68, 0x10, 0x92, 0x0a, 0x25, 0x27, 0x64,
	0xf5, 0x42, 0x12, 0x5c, 0xec, 0xfe, 0x65, 0xa9, 0x45, 0x39, 0x89, 0x95, 0x60, 0x9b, 0x78, 0x82,
	0x60, 0x5c, 0x21, 0x29, 0x2e, 0x8e, 0xd0, 0xbc, 0x14, 0x88, 0x14, 0x13, 0x58, 0x0a, 0xce, 0x57,
	0x92, 0x85, 0x7a, 0x04, 0xbb, 0x8f, 0x9c, 0x24, 0x4e, 0x3c, 0x92, 0x63, 0xbc, 0xf0, 0x48, 0x8e,
	0xf1, 0xc1, 0x23, 0x39, 0xc6, 0x09, 0x8f, 0xe5, 0x18, 0x2e, 0x3c, 0x96, 0x63, 0xb8, 0xf1, 0x58,
	0x8e, 0x21, 0x89, 0x0d, 0x1c, 0x32, 0xc6, 0x80, 0x00, 0x00, 0x00, 0xff, 0xff, 0x45, 0xd0, 0x07,
	0x51, 0x27, 0x01, 0x00, 0x00,
}

func (m *Subscribe) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Subscribe) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Subscribe) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Depth != 0 {
		i = encodeVarintHive(dAtA, i, uint64(m.Depth))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func (m *SubscribeResponse) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *SubscribeResponse) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *SubscribeResponse) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Peers != nil {
		{
			size, err := m.Peers.MarshalToSizedBuffer(dAtA[:i])
			if err != nil {
				return 0, err
			}
			i -= size
			i = encodeVarintHive(dAtA, i, uint64(size))
		}
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *Peers) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Peers) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Peers) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.BzzAddress) > 0 {
		for iNdEx := len(m.BzzAddress) - 1; iNdEx >= 0; iNdEx-- {
			{
				size, err := m.BzzAddress[iNdEx].MarshalToSizedBuffer(dAtA[:i])
				if err != nil {
					return 0, err
				}
				i -= size
				i = encodeVarintHive(dAtA, i, uint64(size))
			}
			i--
			dAtA[i] = 0xa
		}
	}
	return len(dAtA) - i, nil
}

func (m *BzzAddress) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *BzzAddress) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *BzzAddress) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if len(m.Underlay) > 0 {
		i -= len(m.Underlay)
		copy(dAtA[i:], m.Underlay)
		i = encodeVarintHive(dAtA, i, uint64(len(m.Underlay)))
		i--
		dAtA[i] = 0x12
	}
	if len(m.Overlay) > 0 {
		i -= len(m.Overlay)
		copy(dAtA[i:], m.Overlay)
		i = encodeVarintHive(dAtA, i, uint64(len(m.Overlay)))
		i--
		dAtA[i] = 0xa
	}
	return len(dAtA) - i, nil
}

func (m *Depth) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Depth) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Depth) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.Depth != 0 {
		i = encodeVarintHive(dAtA, i, uint64(m.Depth))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func encodeVarintHive(dAtA []byte, offset int, v uint64) int {
	offset -= sovHive(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *Subscribe) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Depth != 0 {
		n += 1 + sovHive(uint64(m.Depth))
	}
	return n
}

func (m *SubscribeResponse) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Peers != nil {
		l = m.Peers.Size()
		n += 1 + l + sovHive(uint64(l))
	}
	return n
}

func (m *Peers) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if len(m.BzzAddress) > 0 {
		for _, e := range m.BzzAddress {
			l = e.Size()
			n += 1 + l + sovHive(uint64(l))
		}
	}
	return n
}

func (m *BzzAddress) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	l = len(m.Overlay)
	if l > 0 {
		n += 1 + l + sovHive(uint64(l))
	}
	l = len(m.Underlay)
	if l > 0 {
		n += 1 + l + sovHive(uint64(l))
	}
	return n
}

func (m *Depth) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.Depth != 0 {
		n += 1 + sovHive(uint64(m.Depth))
	}
	return n
}

func sovHive(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozHive(x uint64) (n int) {
	return sovHive(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Subscribe) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowHive
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Subscribe: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Subscribe: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Depth", wireType)
			}
			m.Depth = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowHive
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Depth |= uint32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipHive(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *SubscribeResponse) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowHive
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: SubscribeResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: SubscribeResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Peers", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowHive
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthHive
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthHive
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Peers == nil {
				m.Peers = &Peers{}
			}
			if err := m.Peers.Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipHive(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Peers) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowHive
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Peers: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Peers: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field BzzAddress", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowHive
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthHive
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthHive
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.BzzAddress = append(m.BzzAddress, &BzzAddress{})
			if err := m.BzzAddress[len(m.BzzAddress)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipHive(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *BzzAddress) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowHive
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: BzzAddress: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: BzzAddress: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Overlay", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowHive
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthHive
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthHive
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Overlay = append(m.Overlay[:0], dAtA[iNdEx:postIndex]...)
			if m.Overlay == nil {
				m.Overlay = []byte{}
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Underlay", wireType)
			}
			var byteLen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowHive
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				byteLen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if byteLen < 0 {
				return ErrInvalidLengthHive
			}
			postIndex := iNdEx + byteLen
			if postIndex < 0 {
				return ErrInvalidLengthHive
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Underlay = append(m.Underlay[:0], dAtA[iNdEx:postIndex]...)
			if m.Underlay == nil {
				m.Underlay = []byte{}
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipHive(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func (m *Depth) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowHive
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: Depth: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Depth: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field Depth", wireType)
			}
			m.Depth = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowHive
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.Depth |= uint32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipHive(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthHive
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func skipHive(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowHive
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowHive
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, ErrIntOverflowHive
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, ErrInvalidLengthHive
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupHive
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthHive
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthHive        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowHive          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupHive = fmt.Errorf("proto: unexpected end of group")
)
