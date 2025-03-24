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
	ReserveSize             uint64            `protobuf:"varint,1,opt,name=ReserveSize,proto3" json:"ReserveSize,omitempty"`
	PullsyncRate            float64           `protobuf:"fixed64,2,opt,name=PullsyncRate,proto3" json:"PullsyncRate,omitempty"`
	StorageRadius           uint32            `protobuf:"varint,3,opt,name=StorageRadius,proto3" json:"StorageRadius,omitempty"`
	ConnectedPeers          uint64            `protobuf:"varint,4,opt,name=ConnectedPeers,proto3" json:"ConnectedPeers,omitempty"`
	NeighborhoodSize        uint64            `protobuf:"varint,5,opt,name=NeighborhoodSize,proto3" json:"NeighborhoodSize,omitempty"`
	BeeMode                 string            `protobuf:"bytes,6,opt,name=BeeMode,proto3" json:"BeeMode,omitempty"`
	BatchCommitment         uint64            `protobuf:"varint,7,opt,name=BatchCommitment,proto3" json:"BatchCommitment,omitempty"`
	IsReachable             bool              `protobuf:"varint,8,opt,name=IsReachable,proto3" json:"IsReachable,omitempty"`
	ReserveSizeWithinRadius uint64            `protobuf:"varint,9,opt,name=ReserveSizeWithinRadius,proto3" json:"ReserveSizeWithinRadius,omitempty"`
	LastSyncedBlock         uint64            `protobuf:"varint,10,opt,name=LastSyncedBlock,proto3" json:"LastSyncedBlock,omitempty"`
	CommittedDepth          uint32            `protobuf:"varint,11,opt,name=CommittedDepth,proto3" json:"CommittedDepth,omitempty"`
	Metrics                 map[string]string `protobuf:"bytes,12,rep,name=Metrics,proto3" json:"Metrics,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
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

func (m *Snapshot) GetCommittedDepth() uint32 {
	if m != nil {
		return m.CommittedDepth
	}
	return 0
}

func (m *Snapshot) GetMetrics() map[string]string {
	if m != nil {
		return m.Metrics
	}
	return nil
}

func init() {
	proto.RegisterType((*Get)(nil), "status.Get")
	proto.RegisterType((*Snapshot)(nil), "status.Snapshot")
	proto.RegisterMapType((map[string]string)(nil), "status.Snapshot.MetricsEntry")
}

func init() { proto.RegisterFile("status.proto", fileDescriptor_dfe4fce6682daf5b) }

var fileDescriptor_dfe4fce6682daf5b = []byte{
	// 400 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x74, 0x92, 0xcf, 0x8a, 0xd4, 0x40,
	0x10, 0xc6, 0xa7, 0xe7, 0xff, 0xd4, 0xcc, 0xea, 0xd2, 0x08, 0x36, 0xa2, 0x21, 0x0c, 0x22, 0xc1,
	0xc3, 0x1c, 0xf4, 0xe0, 0xb2, 0xc7, 0x59, 0x45, 0x04, 0x57, 0x96, 0x9e, 0x83, 0xe0, 0xad, 0x93,
	0x14, 0x9b, 0xb0, 0x99, 0xee, 0x90, 0xae, 0x2c, 0xc4, 0xa7, 0xf0, 0x55, 0x7c, 0x0b, 0x8f, 0x7b,
	0xf4, 0x28, 0x33, 0x2f, 0x22, 0xe9, 0xcc, 0x40, 0x26, 0xe2, 0xad, 0xeb, 0x57, 0x45, 0xf5, 0xd7,
	0xdf, 0xd7, 0xb0, 0xb0, 0xa4, 0xa8, 0xb4, 0xab, 0xbc, 0x30, 0x64, 0xf8, 0xb8, 0xa9, 0x96, 0x23,
	0x18, 0x7c, 0x44, 0x5a, 0xfe, 0x1c, 0xc2, 0x74, 0xa3, 0x55, 0x6e, 0x13, 0x43, 0xdc, 0x87, 0xb9,
	0x44, 0x8b, 0xc5, 0x3d, 0x6e, 0xd2, 0xef, 0x28, 0x98, 0xcf, 0x82, 0xa1, 0x6c, 0x23, 0xbe, 0x84,
	0xc5, 0x4d, 0x99, 0x65, 0xb6, 0xd2, 0x91, 0x54, 0x84, 0xa2, 0xef, 0xb3, 0x80, 0xc9, 0x13, 0xc6,
	0x5f, 0xc2, 0xd9, 0x86, 0x4c, 0xa1, 0x6e, 0x51, 0xaa, 0x38, 0x2d, 0xad, 0x18, 0xf8, 0x2c, 0x38,
	0x93, 0xa7, 0x90, 0xbf, 0x82, 0x47, 0x57, 0x46, 0x6b, 0x8c, 0x08, 0xe3, 0x1b, 0xc4, 0xc2, 0x8a,
	0xa1, 0xbb, 0xae, 0x43, 0xf9, 0x6b, 0x38, 0xff, 0x82, 0xe9, 0x6d, 0x12, 0x9a, 0x22, 0x31, 0x26,
	0x76, 0xc2, 0x46, 0x6e, 0xf2, 0x1f, 0xce, 0x05, 0x4c, 0xd6, 0x88, 0xd7, 0x26, 0x46, 0x31, 0xf6,
	0x59, 0x30, 0x93, 0xc7, 0x92, 0x07, 0xf0, 0x78, 0xad, 0x28, 0x4a, 0xae, 0xcc, 0x76, 0x9b, 0xd2,
	0x16, 0x35, 0x89, 0x89, 0x5b, 0xd2, 0xc5, 0xb5, 0x07, 0x9f, 0xac, 0x44, 0x15, 0x25, 0x2a, 0xcc,
	0x50, 0x4c, 0x7d, 0x16, 0x4c, 0x65, 0x1b, 0xf1, 0x0b, 0x78, 0xda, 0xb2, 0xe4, 0x6b, 0x4a, 0x49,
	0xaa, 0x0f, 0x2f, 0x9d, 0xb9, 0x9d, 0xff, 0x6b, 0xd7, 0x2a, 0x3e, 0x2b, 0x4b, 0x9b, 0x4a, 0x47,
	0x18, 0xaf, 0x33, 0x13, 0xdd, 0x09, 0x68, 0x54, 0x74, 0x70, 0xe3, 0x4e, 0xad, 0x89, 0x30, 0x7e,
	0x8f, 0x39, 0x25, 0x62, 0xee, 0x4c, 0xec, 0x50, 0xfe, 0x0e, 0x26, 0xd7, 0x48, 0x45, 0x1a, 0x59,
	0xb1, 0xf0, 0x07, 0xc1, 0xfc, 0xcd, 0x8b, 0xd5, 0x21, 0xed, 0x63, 0xa8, 0xab, 0x43, 0xff, 0x83,
	0xa6, 0xa2, 0x92, 0xc7, 0xe9, 0x67, 0x97, 0xb0, 0x68, 0x37, 0xf8, 0x39, 0x0c, 0xee, 0xb0, 0x72,
	0x91, 0xcf, 0x64, 0x7d, 0xe4, 0x4f, 0x60, 0x74, 0xaf, 0xb2, 0xb2, 0xc9, 0x78, 0x26, 0x9b, 0xe2,
	0xb2, 0x7f, 0xc1, 0xd6, 0xcf, 0x7f, 0xed, 0x3c, 0xf6, 0xb0, 0xf3, 0xd8, 0x9f, 0x9d, 0xc7, 0x7e,
	0xec, 0xbd, 0xde, 0xc3, 0xde, 0xeb, 0xfd, 0xde, 0x7b, 0xbd, 0x6f, 0xfd, 0x3c, 0x0c, 0xc7, 0xee,
	0x9f, 0xbd, 0xfd, 0x1b, 0x00, 0x00, 0xff, 0xff, 0x23, 0x3c, 0xa8, 0x9b, 0x77, 0x02, 0x00, 0x00,
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
	if len(m.Metrics) > 0 {
		for k := range m.Metrics {
			v := m.Metrics[k]
			baseI := i
			i -= len(v)
			copy(dAtA[i:], v)
			i = encodeVarintStatus(dAtA, i, uint64(len(v)))
			i--
			dAtA[i] = 0x12
			i -= len(k)
			copy(dAtA[i:], k)
			i = encodeVarintStatus(dAtA, i, uint64(len(k)))
			i--
			dAtA[i] = 0xa
			i = encodeVarintStatus(dAtA, i, uint64(baseI-i))
			i--
			dAtA[i] = 0x62
		}
	}
	if m.CommittedDepth != 0 {
		i = encodeVarintStatus(dAtA, i, uint64(m.CommittedDepth))
		i--
		dAtA[i] = 0x58
	}
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
	if m.CommittedDepth != 0 {
		n += 1 + sovStatus(uint64(m.CommittedDepth))
	}
	if len(m.Metrics) > 0 {
		for k, v := range m.Metrics {
			_ = k
			_ = v
			mapEntrySize := 1 + len(k) + sovStatus(uint64(len(k))) + 1 + len(v) + sovStatus(uint64(len(v)))
			n += mapEntrySize + 1 + sovStatus(uint64(mapEntrySize))
		}
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
			if (skippy < 0) || (iNdEx+skippy) < 0 {
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
		case 11:
			if wireType != 0 {
				return fmt.Errorf("proto: wrong wireType = %d for field CommittedDepth", wireType)
			}
			m.CommittedDepth = 0
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				m.CommittedDepth |= uint32(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
		case 12:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Metrics", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowStatus
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
				return ErrInvalidLengthStatus
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthStatus
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			if m.Metrics == nil {
				m.Metrics = make(map[string]string)
			}
			var mapkey string
			var mapvalue string
			for iNdEx < postIndex {
				entryPreIndex := iNdEx
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
				if fieldNum == 1 {
					var stringLenmapkey uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflowStatus
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						stringLenmapkey |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapkey := int(stringLenmapkey)
					if intStringLenmapkey < 0 {
						return ErrInvalidLengthStatus
					}
					postStringIndexmapkey := iNdEx + intStringLenmapkey
					if postStringIndexmapkey < 0 {
						return ErrInvalidLengthStatus
					}
					if postStringIndexmapkey > l {
						return io.ErrUnexpectedEOF
					}
					mapkey = string(dAtA[iNdEx:postStringIndexmapkey])
					iNdEx = postStringIndexmapkey
				} else if fieldNum == 2 {
					var stringLenmapvalue uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return ErrIntOverflowStatus
						}
						if iNdEx >= l {
							return io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						stringLenmapvalue |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapvalue := int(stringLenmapvalue)
					if intStringLenmapvalue < 0 {
						return ErrInvalidLengthStatus
					}
					postStringIndexmapvalue := iNdEx + intStringLenmapvalue
					if postStringIndexmapvalue < 0 {
						return ErrInvalidLengthStatus
					}
					if postStringIndexmapvalue > l {
						return io.ErrUnexpectedEOF
					}
					mapvalue = string(dAtA[iNdEx:postStringIndexmapvalue])
					iNdEx = postStringIndexmapvalue
				} else {
					iNdEx = entryPreIndex
					skippy, err := skipStatus(dAtA[iNdEx:])
					if err != nil {
						return err
					}
					if (skippy < 0) || (iNdEx+skippy) < 0 {
						return ErrInvalidLengthStatus
					}
					if (iNdEx + skippy) > postIndex {
						return io.ErrUnexpectedEOF
					}
					iNdEx += skippy
				}
			}
			m.Metrics[mapkey] = mapvalue
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipStatus(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
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
