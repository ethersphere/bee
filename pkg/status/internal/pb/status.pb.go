// Code generated by protoc-gen-gogo. DO NOT EDIT.
// source: status.proto

package pb

import (
	encoding_binary "encoding/binary"
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

// Get message indicate interest in receiving a node Snapshot.
type Get struct {
}

func (m *Get) Reset()         { *m = Get{} }
func (m *Get) String() string { return proto.CompactTextString(m) }
func (*Get) ProtoMessage()    {}
func (*Get) Descriptor() ([]byte, []int) {
	return fileDescriptor_dfe4fce6682daf5b, []int{0}
}
func (m *Get) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Get) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Get.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Get) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Get.Merge(m, src)
}
func (m *Get) XXX_Size() int {
	return m.Size()
}
func (m *Get) XXX_DiscardUnknown() {
	xxx_messageInfo_Get.DiscardUnknown(m)
}

var xxx_messageInfo_Get proto.InternalMessageInfo

// Snapshot message is a response to the Get message and contains
// the appropriate values that are a snapshot of the current state
// of the running node.
type Snapshot struct {
	ReserveSize             uint64  `protobuf:"varint,1,opt,name=ReserveSize,proto3" json:"ReserveSize,omitempty"`
	PullsyncRate            float64 `protobuf:"fixed64,2,opt,name=PullsyncRate,proto3" json:"PullsyncRate,omitempty"`
	StorageRadius           uint32  `protobuf:"varint,3,opt,name=StorageRadius,proto3" json:"StorageRadius,omitempty"`
	ConnectedPeers          uint64  `protobuf:"varint,4,opt,name=ConnectedPeers,proto3" json:"ConnectedPeers,omitempty"`
	NeighborhoodSize        uint64  `protobuf:"varint,5,opt,name=NeighborhoodSize,proto3" json:"NeighborhoodSize,omitempty"`
	BeeMode                 string  `protobuf:"bytes,6,opt,name=BeeMode,proto3" json:"BeeMode,omitempty"`
	BatchCommitment         uint64  `protobuf:"varint,7,opt,name=BatchCommitment,proto3" json:"BatchCommitment,omitempty"`
	IsReachable             bool    `protobuf:"varint,8,opt,name=IsReachable,proto3" json:"IsReachable,omitempty"`
	ReserveSizeWithinRadius uint64  `protobuf:"varint,9,opt,name=ReserveSizeWithinRadius,proto3" json:"ReserveSizeWithinRadius,omitempty"`
	LastSyncedBlock         uint64  `protobuf:"varint,10,opt,name=LastSyncedBlock,proto3" json:"LastSyncedBlock,omitempty"`
}

func (m *Snapshot) Reset()         { *m = Snapshot{} }
func (m *Snapshot) String() string { return proto.CompactTextString(m) }
func (*Snapshot) ProtoMessage()    {}
func (*Snapshot) Descriptor() ([]byte, []int) {
	return fileDescriptor_dfe4fce6682daf5b, []int{1}
}
func (m *Snapshot) XXX_Unmarshal(b []byte) error {
	return m.Unmarshal(b)
}
func (m *Snapshot) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	if deterministic {
		return xxx_messageInfo_Snapshot.Marshal(b, m, deterministic)
	} else {
		b = b[:cap(b)]
		n, err := m.MarshalToSizedBuffer(b)
		if err != nil {
			return nil, err
		}
		return b[:n], nil
	}
}
func (m *Snapshot) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Snapshot.Merge(m, src)
}
func (m *Snapshot) XXX_Size() int {
	return m.Size()
}
func (m *Snapshot) XXX_DiscardUnknown() {
	xxx_messageInfo_Snapshot.DiscardUnknown(m)
}

var xxx_messageInfo_Snapshot proto.InternalMessageInfo

func (m *Snapshot) GetReserveSize() uint64 {
	if m != nil {
		return m.ReserveSize
	}
	return 0
}

func (m *Snapshot) GetPullsyncRate() float64 {
	if m != nil {
		return m.PullsyncRate
	}
	return 0
}

func (m *Snapshot) GetStorageRadius() uint32 {
	if m != nil {
		return m.StorageRadius
	}
	return 0
}

func (m *Snapshot) GetConnectedPeers() uint64 {
	if m != nil {
		return m.ConnectedPeers
	}
	return 0
}

func (m *Snapshot) GetNeighborhoodSize() uint64 {
	if m != nil {
		return m.NeighborhoodSize
	}
	return 0
}

func (m *Snapshot) GetBeeMode() string {
	if m != nil {
		return m.BeeMode
	}
	return ""
}

func (m *Snapshot) GetBatchCommitment() uint64 {
	if m != nil {
		return m.BatchCommitment
	}
	return 0
}

func (m *Snapshot) GetIsReachable() bool {
	if m != nil {
		return m.IsReachable
	}
	return false
}

func (m *Snapshot) GetReserveSizeWithinRadius() uint64 {
	if m != nil {
		return m.ReserveSizeWithinRadius
	}
	return 0
}

func (m *Snapshot) GetLastSyncedBlock() uint64 {
	if m != nil {
		return m.LastSyncedBlock
	}
	return 0
}

func init() {
	proto.RegisterType((*Get)(nil), "status.Get")
	proto.RegisterType((*Snapshot)(nil), "status.Snapshot")
}

func init() { proto.RegisterFile("status.proto", fileDescriptor_dfe4fce6682daf5b) }

var fileDescriptor_dfe4fce6682daf5b = []byte{
	// 317 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0xd1, 0xbd, 0x4a, 0x03, 0x41,
	0x10, 0x07, 0xf0, 0x6c, 0xbe, 0x33, 0x26, 0x2a, 0xdb, 0xb8, 0x85, 0x1c, 0x47, 0x10, 0x39, 0x2c,
	0x6c, 0x6c, 0xac, 0x2f, 0x85, 0x08, 0x2a, 0x61, 0xaf, 0x10, 0xec, 0xf6, 0xee, 0x86, 0xdc, 0xe1,
	0x65, 0x37, 0xdc, 0x4e, 0x84, 0xf8, 0x14, 0x16, 0x3e, 0x94, 0x65, 0x4a, 0x4b, 0x49, 0x5e, 0x44,
	0xb2, 0x51, 0x48, 0x4e, 0x2c, 0xe7, 0xb7, 0xcb, 0xec, 0x7f, 0x67, 0xa0, 0x6f, 0x49, 0xd1, 0xdc,
	0x5e, 0xce, 0x4a, 0x43, 0x86, 0xb7, 0xb7, 0xd5, 0xb0, 0x05, 0x8d, 0x1b, 0xa4, 0xe1, 0x7b, 0x03,
	0xba, 0x91, 0x56, 0x33, 0x9b, 0x19, 0xe2, 0x3e, 0x1c, 0x48, 0xb4, 0x58, 0xbe, 0x60, 0x94, 0xbf,
	0xa2, 0x60, 0x3e, 0x0b, 0x9a, 0x72, 0x97, 0xf8, 0x10, 0xfa, 0xe3, 0x79, 0x51, 0xd8, 0x85, 0x4e,
	0xa4, 0x22, 0x14, 0x75, 0x9f, 0x05, 0x4c, 0xee, 0x19, 0x3f, 0x83, 0x41, 0x44, 0xa6, 0x54, 0x13,
	0x94, 0x2a, 0xcd, 0xe7, 0x56, 0x34, 0x7c, 0x16, 0x0c, 0xe4, 0x3e, 0xf2, 0x73, 0x38, 0x1c, 0x19,
	0xad, 0x31, 0x21, 0x4c, 0xc7, 0x88, 0xa5, 0x15, 0x4d, 0xf7, 0x5c, 0x45, 0xf9, 0x05, 0x1c, 0x3f,
	0x60, 0x3e, 0xc9, 0x62, 0x53, 0x66, 0xc6, 0xa4, 0x2e, 0x58, 0xcb, 0xdd, 0xfc, 0xe3, 0x5c, 0x40,
	0x27, 0x44, 0xbc, 0x37, 0x29, 0x8a, 0xb6, 0xcf, 0x82, 0x9e, 0xfc, 0x2d, 0x79, 0x00, 0x47, 0xa1,
	0xa2, 0x24, 0x1b, 0x99, 0xe9, 0x34, 0xa7, 0x29, 0x6a, 0x12, 0x1d, 0xd7, 0xa4, 0xca, 0x9b, 0x19,
	0xdc, 0x5a, 0x89, 0x2a, 0xc9, 0x54, 0x5c, 0xa0, 0xe8, 0xfa, 0x2c, 0xe8, 0xca, 0x5d, 0xe2, 0xd7,
	0x70, 0xb2, 0x33, 0x92, 0xc7, 0x9c, 0xb2, 0x5c, 0xff, 0xfc, 0xb4, 0xe7, 0x7a, 0xfe, 0x77, 0xbc,
	0x49, 0x71, 0xa7, 0x2c, 0x45, 0x0b, 0x9d, 0x60, 0x1a, 0x16, 0x26, 0x79, 0x16, 0xb0, 0x4d, 0x51,
	0xe1, 0xf0, 0xf4, 0x63, 0xe5, 0xb1, 0xe5, 0xca, 0x63, 0x5f, 0x2b, 0x8f, 0xbd, 0xad, 0xbd, 0xda,
	0x72, 0xed, 0xd5, 0x3e, 0xd7, 0x5e, 0xed, 0xa9, 0x3e, 0x8b, 0xe3, 0xb6, 0x5b, 0xe5, 0xd5, 0x77,
	0x00, 0x00, 0x00, 0xff, 0xff, 0x92, 0x33, 0xb8, 0x1e, 0xda, 0x01, 0x00, 0x00,
}

func (m *Get) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Get) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Get) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	return len(dAtA) - i, nil
}

func (m *Snapshot) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *Snapshot) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

func (m *Snapshot) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	_ = i
	var l int
	_ = l
	if m.LastSyncedBlock != 0 {
		i = encodeVarintStatus(dAtA, i, uint64(m.LastSyncedBlock))
		i--
		dAtA[i] = 0x50
	}
	if m.ReserveSizeWithinRadius != 0 {
		i = encodeVarintStatus(dAtA, i, uint64(m.ReserveSizeWithinRadius))
		i--
		dAtA[i] = 0x48
	}
	if m.IsReachable {
		i--
		if m.IsReachable {
			dAtA[i] = 1
		} else {
			dAtA[i] = 0
		}
		i--
		dAtA[i] = 0x40
	}
	if m.BatchCommitment != 0 {
		i = encodeVarintStatus(dAtA, i, uint64(m.BatchCommitment))
		i--
		dAtA[i] = 0x38
	}
	if len(m.BeeMode) > 0 {
		i -= len(m.BeeMode)
		copy(dAtA[i:], m.BeeMode)
		i = encodeVarintStatus(dAtA, i, uint64(len(m.BeeMode)))
		i--
		dAtA[i] = 0x32
	}
	if m.NeighborhoodSize != 0 {
		i = encodeVarintStatus(dAtA, i, uint64(m.NeighborhoodSize))
		i--
		dAtA[i] = 0x28
	}
	if m.ConnectedPeers != 0 {
		i = encodeVarintStatus(dAtA, i, uint64(m.ConnectedPeers))
		i--
		dAtA[i] = 0x20
	}
	if m.StorageRadius != 0 {
		i = encodeVarintStatus(dAtA, i, uint64(m.StorageRadius))
		i--
		dAtA[i] = 0x18
	}
	if m.PullsyncRate != 0 {
		i -= 8
		encoding_binary.LittleEndian.PutUint64(dAtA[i:], uint64(math.Float64bits(float64(m.PullsyncRate))))
		i--
		dAtA[i] = 0x11
	}
	if m.ReserveSize != 0 {
		i = encodeVarintStatus(dAtA, i, uint64(m.ReserveSize))
		i--
		dAtA[i] = 0x8
	}
	return len(dAtA) - i, nil
}

func encodeVarintStatus(dAtA []byte, offset int, v uint64) int {
	offset -= sovStatus(v)
	base := offset
	for v >= 1<<7 {
		dAtA[offset] = uint8(v&0x7f | 0x80)
		v >>= 7
		offset++
	}
	dAtA[offset] = uint8(v)
	return base
}
func (m *Get) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	return n
}

func (m *Snapshot) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	_ = l
	if m.ReserveSize != 0 {
		n += 1 + sovStatus(uint64(m.ReserveSize))
	}
	if m.PullsyncRate != 0 {
		n += 9
	}
	if m.StorageRadius != 0 {
		n += 1 + sovStatus(uint64(m.StorageRadius))
	}
	if m.ConnectedPeers != 0 {
		n += 1 + sovStatus(uint64(m.ConnectedPeers))
	}
	if m.NeighborhoodSize != 0 {
		n += 1 + sovStatus(uint64(m.NeighborhoodSize))
	}
	l = len(m.BeeMode)
	if l > 0 {
		n += 1 + l + sovStatus(uint64(l))
	}
	if m.BatchCommitment != 0 {
		n += 1 + sovStatus(uint64(m.BatchCommitment))
	}
	if m.IsReachable {
		n += 2
	}
	if m.ReserveSizeWithinRadius != 0 {
		n += 1 + sovStatus(uint64(m.ReserveSizeWithinRadius))
	}
	if m.LastSyncedBlock != 0 {
		n += 1 + sovStatus(uint64(m.LastSyncedBlock))
	}
	return n
}

func sovStatus(x uint64) (n int) {
	return (math_bits.Len64(x|1) + 6) / 7
}
func sozStatus(x uint64) (n int) {
	return sovStatus(uint64((x << 1) ^ uint64((int64(x) >> 63))))
}
func (m *Get) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowStatus
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
			return fmt.Errorf("proto: Get: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Get: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		default:
			iNdEx = preIndex
			skippy, err := skipStatus(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthStatus
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthStatus
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
func (m *Snapshot) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowStatus
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
			return fmt.Errorf("proto: Snapshot: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: Snapshot: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field ReserveSize", wireType)
			}
			m.ReserveSize = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.ReserveSize |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 2:
			if wireType != 1 {
				return fmt.Errorf("proto: wrong wireType = %d for field PullsyncRate", wireType)
			}
			var v uint64
			if (iNdEx + 8) > l {
				return io.ErrUnexpectedEOF
			}
			v = uint64(encoding_binary.LittleEndian.Uint64(dAtA[iNdEx:]))
			iNdEx += 8
			m.PullsyncRate = float64(math.Float64frombits(v))
		case 3:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field StorageRadius", wireType)
			}
			m.StorageRadius = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.StorageRadius |= uint32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 4:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field ConnectedPeers", wireType)
			}
			m.ConnectedPeers = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.ConnectedPeers |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 5:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field NeighborhoodSize", wireType)
			}
			m.NeighborhoodSize = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.NeighborhoodSize |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 6:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field BeeMode", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthStatus
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthStatus
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.BeeMode = string(dAtA[iNdEx:postIndex])
			iNdEx = postIndex
		case 7:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field BatchCommitment", wireType)
			}
			m.BatchCommitment = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.BatchCommitment |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 8:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field IsReachable", wireType)
			}
			var v int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				v |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			m.IsReachable = bool(v != 0)
		case 9:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field ReserveSizeWithinRadius", wireType)
			}
			m.ReserveSizeWithinRadius = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.ReserveSizeWithinRadius |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 10:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field LastSyncedBlock", wireType)
			}
			m.LastSyncedBlock = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.LastSyncedBlock |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		default:
			iNdEx = preIndex
			skippy, err := skipStatus(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthStatus
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthStatus
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
func skipStatus(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, ErrIntOverflowStatus
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
					return 0, ErrIntOverflowStatus
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
					return 0, ErrIntOverflowStatus
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
				return 0, ErrInvalidLengthStatus
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, ErrUnexpectedEndOfGroupStatus
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, ErrInvalidLengthStatus
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}

var (
	ErrInvalidLengthStatus        = fmt.Errorf("proto: negative length found during unmarshaling")
	ErrIntOverflowStatus          = fmt.Errorf("proto: integer overflow")
	ErrUnexpectedEndOfGroupStatus = fmt.Errorf("proto: unexpected end of group")
)
