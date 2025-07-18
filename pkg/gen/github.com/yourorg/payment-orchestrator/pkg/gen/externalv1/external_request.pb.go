// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.1
// 	protoc        (unknown)
// source: schemas/external/v1/external_request.proto

package externalv1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type ExternalRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	RequestId     string            `protobuf:"bytes,1,opt,name=request_id,json=requestId,proto3" json:"request_id,omitempty"`
	MerchantId    string            `protobuf:"bytes,2,opt,name=merchant_id,json=merchantId,proto3" json:"merchant_id,omitempty"`
	Amount        int64             `protobuf:"varint,3,opt,name=amount,proto3" json:"amount,omitempty"`
	Currency      string            `protobuf:"bytes,4,opt,name=currency,proto3" json:"currency,omitempty"`
	Customer      *CustomerDetails  `protobuf:"bytes,5,opt,name=customer,proto3" json:"customer,omitempty"`
	PaymentMethod *PaymentMethod    `protobuf:"bytes,6,opt,name=payment_method,json=paymentMethod,proto3" json:"payment_method,omitempty"`
	Metadata      map[string]string `protobuf:"bytes,7,rep,name=metadata,proto3" json:"metadata,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (x *ExternalRequest) Reset() {
	*x = ExternalRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_schemas_external_v1_external_request_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ExternalRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ExternalRequest) ProtoMessage() {}

func (x *ExternalRequest) ProtoReflect() protoreflect.Message {
	mi := &file_schemas_external_v1_external_request_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ExternalRequest.ProtoReflect.Descriptor instead.
func (*ExternalRequest) Descriptor() ([]byte, []int) {
	return file_schemas_external_v1_external_request_proto_rawDescGZIP(), []int{0}
}

func (x *ExternalRequest) GetRequestId() string {
	if x != nil {
		return x.RequestId
	}
	return ""
}

func (x *ExternalRequest) GetMerchantId() string {
	if x != nil {
		return x.MerchantId
	}
	return ""
}

func (x *ExternalRequest) GetAmount() int64 {
	if x != nil {
		return x.Amount
	}
	return 0
}

func (x *ExternalRequest) GetCurrency() string {
	if x != nil {
		return x.Currency
	}
	return ""
}

func (x *ExternalRequest) GetCustomer() *CustomerDetails {
	if x != nil {
		return x.Customer
	}
	return nil
}

func (x *ExternalRequest) GetPaymentMethod() *PaymentMethod {
	if x != nil {
		return x.PaymentMethod
	}
	return nil
}

func (x *ExternalRequest) GetMetadata() map[string]string {
	if x != nil {
		return x.Metadata
	}
	return nil
}

type CustomerDetails struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Name  string `protobuf:"bytes,1,opt,name=name,proto3" json:"name,omitempty"`
	Email string `protobuf:"bytes,2,opt,name=email,proto3" json:"email,omitempty"`
	Phone string `protobuf:"bytes,3,opt,name=phone,proto3" json:"phone,omitempty"`
}

func (x *CustomerDetails) Reset() {
	*x = CustomerDetails{}
	if protoimpl.UnsafeEnabled {
		mi := &file_schemas_external_v1_external_request_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CustomerDetails) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CustomerDetails) ProtoMessage() {}

func (x *CustomerDetails) ProtoReflect() protoreflect.Message {
	mi := &file_schemas_external_v1_external_request_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CustomerDetails.ProtoReflect.Descriptor instead.
func (*CustomerDetails) Descriptor() ([]byte, []int) {
	return file_schemas_external_v1_external_request_proto_rawDescGZIP(), []int{1}
}

func (x *CustomerDetails) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *CustomerDetails) GetEmail() string {
	if x != nil {
		return x.Email
	}
	return ""
}

func (x *CustomerDetails) GetPhone() string {
	if x != nil {
		return x.Phone
	}
	return ""
}

type PaymentMethod struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to Method:
	//
	//	*PaymentMethod_Card
	//	*PaymentMethod_Wallet
	Method isPaymentMethod_Method `protobuf_oneof:"method"`
}

func (x *PaymentMethod) Reset() {
	*x = PaymentMethod{}
	if protoimpl.UnsafeEnabled {
		mi := &file_schemas_external_v1_external_request_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *PaymentMethod) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*PaymentMethod) ProtoMessage() {}

func (x *PaymentMethod) ProtoReflect() protoreflect.Message {
	mi := &file_schemas_external_v1_external_request_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use PaymentMethod.ProtoReflect.Descriptor instead.
func (*PaymentMethod) Descriptor() ([]byte, []int) {
	return file_schemas_external_v1_external_request_proto_rawDescGZIP(), []int{2}
}

func (m *PaymentMethod) GetMethod() isPaymentMethod_Method {
	if m != nil {
		return m.Method
	}
	return nil
}

func (x *PaymentMethod) GetCard() *CardInfo {
	if x, ok := x.GetMethod().(*PaymentMethod_Card); ok {
		return x.Card
	}
	return nil
}

func (x *PaymentMethod) GetWallet() *WalletInfo {
	if x, ok := x.GetMethod().(*PaymentMethod_Wallet); ok {
		return x.Wallet
	}
	return nil
}

type isPaymentMethod_Method interface {
	isPaymentMethod_Method()
}

type PaymentMethod_Card struct {
	Card *CardInfo `protobuf:"bytes,1,opt,name=card,proto3,oneof"`
}

type PaymentMethod_Wallet struct {
	Wallet *WalletInfo `protobuf:"bytes,2,opt,name=wallet,proto3,oneof"`
}

func (*PaymentMethod_Card) isPaymentMethod_Method() {}

func (*PaymentMethod_Wallet) isPaymentMethod_Method() {}

type CardInfo struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	CardNumber  string `protobuf:"bytes,1,opt,name=card_number,json=cardNumber,proto3" json:"card_number,omitempty"`
	ExpiryMonth string `protobuf:"bytes,2,opt,name=expiry_month,json=expiryMonth,proto3" json:"expiry_month,omitempty"`
	ExpiryYear  string `protobuf:"bytes,3,opt,name=expiry_year,json=expiryYear,proto3" json:"expiry_year,omitempty"`
	Cvv         string `protobuf:"bytes,4,opt,name=cvv,proto3" json:"cvv,omitempty"`
}

func (x *CardInfo) Reset() {
	*x = CardInfo{}
	if protoimpl.UnsafeEnabled {
		mi := &file_schemas_external_v1_external_request_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *CardInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*CardInfo) ProtoMessage() {}

func (x *CardInfo) ProtoReflect() protoreflect.Message {
	mi := &file_schemas_external_v1_external_request_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use CardInfo.ProtoReflect.Descriptor instead.
func (*CardInfo) Descriptor() ([]byte, []int) {
	return file_schemas_external_v1_external_request_proto_rawDescGZIP(), []int{3}
}

func (x *CardInfo) GetCardNumber() string {
	if x != nil {
		return x.CardNumber
	}
	return ""
}

func (x *CardInfo) GetExpiryMonth() string {
	if x != nil {
		return x.ExpiryMonth
	}
	return ""
}

func (x *CardInfo) GetExpiryYear() string {
	if x != nil {
		return x.ExpiryYear
	}
	return ""
}

func (x *CardInfo) GetCvv() string {
	if x != nil {
		return x.Cvv
	}
	return ""
}

type WalletInfo struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	WalletType string `protobuf:"bytes,1,opt,name=wallet_type,json=walletType,proto3" json:"wallet_type,omitempty"` // e.g., "ApplePay", "GooglePay"
	Token      string `protobuf:"bytes,2,opt,name=token,proto3" json:"token,omitempty"`
}

func (x *WalletInfo) Reset() {
	*x = WalletInfo{}
	if protoimpl.UnsafeEnabled {
		mi := &file_schemas_external_v1_external_request_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *WalletInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*WalletInfo) ProtoMessage() {}

func (x *WalletInfo) ProtoReflect() protoreflect.Message {
	mi := &file_schemas_external_v1_external_request_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use WalletInfo.ProtoReflect.Descriptor instead.
func (*WalletInfo) Descriptor() ([]byte, []int) {
	return file_schemas_external_v1_external_request_proto_rawDescGZIP(), []int{4}
}

func (x *WalletInfo) GetWalletType() string {
	if x != nil {
		return x.WalletType
	}
	return ""
}

func (x *WalletInfo) GetToken() string {
	if x != nil {
		return x.Token
	}
	return ""
}

var File_schemas_external_v1_external_request_proto protoreflect.FileDescriptor

var file_schemas_external_v1_external_request_proto_rawDesc = []byte{
	0x0a, 0x2a, 0x73, 0x63, 0x68, 0x65, 0x6d, 0x61, 0x73, 0x2f, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2f, 0x76, 0x31, 0x2f, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x5f, 0x72,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x0b, 0x65, 0x78,
	0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x76, 0x31, 0x22, 0x87, 0x03, 0x0a, 0x0f, 0x45, 0x78,
	0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x1d, 0x0a,
	0x0a, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x09, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x49, 0x64, 0x12, 0x1f, 0x0a, 0x0b,
	0x6d, 0x65, 0x72, 0x63, 0x68, 0x61, 0x6e, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0a, 0x6d, 0x65, 0x72, 0x63, 0x68, 0x61, 0x6e, 0x74, 0x49, 0x64, 0x12, 0x16, 0x0a,
	0x06, 0x61, 0x6d, 0x6f, 0x75, 0x6e, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x03, 0x52, 0x06, 0x61,
	0x6d, 0x6f, 0x75, 0x6e, 0x74, 0x12, 0x1a, 0x0a, 0x08, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x63,
	0x79, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x63, 0x75, 0x72, 0x72, 0x65, 0x6e, 0x63,
	0x79, 0x12, 0x38, 0x0a, 0x08, 0x63, 0x75, 0x73, 0x74, 0x6f, 0x6d, 0x65, 0x72, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x1c, 0x2e, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x76,
	0x31, 0x2e, 0x43, 0x75, 0x73, 0x74, 0x6f, 0x6d, 0x65, 0x72, 0x44, 0x65, 0x74, 0x61, 0x69, 0x6c,
	0x73, 0x52, 0x08, 0x63, 0x75, 0x73, 0x74, 0x6f, 0x6d, 0x65, 0x72, 0x12, 0x41, 0x0a, 0x0e, 0x70,
	0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x5f, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x18, 0x06, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x76,
	0x31, 0x2e, 0x50, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x52,
	0x0d, 0x70, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x12, 0x46,
	0x0a, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x18, 0x07, 0x20, 0x03, 0x28, 0x0b,
	0x32, 0x2a, 0x2e, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e, 0x76, 0x31, 0x2e, 0x45,
	0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x2e, 0x4d,
	0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x08, 0x6d, 0x65,
	0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x1a, 0x3b, 0x0a, 0x0d, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61,
	0x74, 0x61, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c,
	0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a,
	0x02, 0x38, 0x01, 0x22, 0x51, 0x0a, 0x0f, 0x43, 0x75, 0x73, 0x74, 0x6f, 0x6d, 0x65, 0x72, 0x44,
	0x65, 0x74, 0x61, 0x69, 0x6c, 0x73, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x65, 0x6d,
	0x61, 0x69, 0x6c, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x65, 0x6d, 0x61, 0x69, 0x6c,
	0x12, 0x14, 0x0a, 0x05, 0x70, 0x68, 0x6f, 0x6e, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x05, 0x70, 0x68, 0x6f, 0x6e, 0x65, 0x22, 0x79, 0x0a, 0x0d, 0x50, 0x61, 0x79, 0x6d, 0x65, 0x6e,
	0x74, 0x4d, 0x65, 0x74, 0x68, 0x6f, 0x64, 0x12, 0x2b, 0x0a, 0x04, 0x63, 0x61, 0x72, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x15, 0x2e, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c,
	0x2e, 0x76, 0x31, 0x2e, 0x43, 0x61, 0x72, 0x64, 0x49, 0x6e, 0x66, 0x6f, 0x48, 0x00, 0x52, 0x04,
	0x63, 0x61, 0x72, 0x64, 0x12, 0x31, 0x0a, 0x06, 0x77, 0x61, 0x6c, 0x6c, 0x65, 0x74, 0x18, 0x02,
	0x20, 0x01, 0x28, 0x0b, 0x32, 0x17, 0x2e, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2e,
	0x76, 0x31, 0x2e, 0x57, 0x61, 0x6c, 0x6c, 0x65, 0x74, 0x49, 0x6e, 0x66, 0x6f, 0x48, 0x00, 0x52,
	0x06, 0x77, 0x61, 0x6c, 0x6c, 0x65, 0x74, 0x42, 0x08, 0x0a, 0x06, 0x6d, 0x65, 0x74, 0x68, 0x6f,
	0x64, 0x22, 0x81, 0x01, 0x0a, 0x08, 0x43, 0x61, 0x72, 0x64, 0x49, 0x6e, 0x66, 0x6f, 0x12, 0x1f,
	0x0a, 0x0b, 0x63, 0x61, 0x72, 0x64, 0x5f, 0x6e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0a, 0x63, 0x61, 0x72, 0x64, 0x4e, 0x75, 0x6d, 0x62, 0x65, 0x72, 0x12,
	0x21, 0x0a, 0x0c, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x5f, 0x6d, 0x6f, 0x6e, 0x74, 0x68, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x4d, 0x6f, 0x6e,
	0x74, 0x68, 0x12, 0x1f, 0x0a, 0x0b, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x5f, 0x79, 0x65, 0x61,
	0x72, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x65, 0x78, 0x70, 0x69, 0x72, 0x79, 0x59,
	0x65, 0x61, 0x72, 0x12, 0x10, 0x0a, 0x03, 0x63, 0x76, 0x76, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x03, 0x63, 0x76, 0x76, 0x22, 0x43, 0x0a, 0x0a, 0x57, 0x61, 0x6c, 0x6c, 0x65, 0x74, 0x49,
	0x6e, 0x66, 0x6f, 0x12, 0x1f, 0x0a, 0x0b, 0x77, 0x61, 0x6c, 0x6c, 0x65, 0x74, 0x5f, 0x74, 0x79,
	0x70, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0a, 0x77, 0x61, 0x6c, 0x6c, 0x65, 0x74,
	0x54, 0x79, 0x70, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x42, 0x47, 0x5a, 0x45, 0x67, 0x69,
	0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x79, 0x6f, 0x75, 0x72, 0x6f, 0x72, 0x67,
	0x2f, 0x70, 0x61, 0x79, 0x6d, 0x65, 0x6e, 0x74, 0x2d, 0x6f, 0x72, 0x63, 0x68, 0x65, 0x73, 0x74,
	0x72, 0x61, 0x74, 0x6f, 0x72, 0x2f, 0x70, 0x6b, 0x67, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x65, 0x78,
	0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x76, 0x31, 0x3b, 0x65, 0x78, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x76, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_schemas_external_v1_external_request_proto_rawDescOnce sync.Once
	file_schemas_external_v1_external_request_proto_rawDescData = file_schemas_external_v1_external_request_proto_rawDesc
)

func file_schemas_external_v1_external_request_proto_rawDescGZIP() []byte {
	file_schemas_external_v1_external_request_proto_rawDescOnce.Do(func() {
		file_schemas_external_v1_external_request_proto_rawDescData = protoimpl.X.CompressGZIP(file_schemas_external_v1_external_request_proto_rawDescData)
	})
	return file_schemas_external_v1_external_request_proto_rawDescData
}

var file_schemas_external_v1_external_request_proto_msgTypes = make([]protoimpl.MessageInfo, 6)
var file_schemas_external_v1_external_request_proto_goTypes = []interface{}{
	(*ExternalRequest)(nil), // 0: external.v1.ExternalRequest
	(*CustomerDetails)(nil), // 1: external.v1.CustomerDetails
	(*PaymentMethod)(nil),   // 2: external.v1.PaymentMethod
	(*CardInfo)(nil),        // 3: external.v1.CardInfo
	(*WalletInfo)(nil),      // 4: external.v1.WalletInfo
	nil,                     // 5: external.v1.ExternalRequest.MetadataEntry
}
var file_schemas_external_v1_external_request_proto_depIdxs = []int32{
	1, // 0: external.v1.ExternalRequest.customer:type_name -> external.v1.CustomerDetails
	2, // 1: external.v1.ExternalRequest.payment_method:type_name -> external.v1.PaymentMethod
	5, // 2: external.v1.ExternalRequest.metadata:type_name -> external.v1.ExternalRequest.MetadataEntry
	3, // 3: external.v1.PaymentMethod.card:type_name -> external.v1.CardInfo
	4, // 4: external.v1.PaymentMethod.wallet:type_name -> external.v1.WalletInfo
	5, // [5:5] is the sub-list for method output_type
	5, // [5:5] is the sub-list for method input_type
	5, // [5:5] is the sub-list for extension type_name
	5, // [5:5] is the sub-list for extension extendee
	0, // [0:5] is the sub-list for field type_name
}

func init() { file_schemas_external_v1_external_request_proto_init() }
func file_schemas_external_v1_external_request_proto_init() {
	if File_schemas_external_v1_external_request_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_schemas_external_v1_external_request_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ExternalRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_schemas_external_v1_external_request_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CustomerDetails); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_schemas_external_v1_external_request_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*PaymentMethod); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_schemas_external_v1_external_request_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*CardInfo); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_schemas_external_v1_external_request_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*WalletInfo); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	file_schemas_external_v1_external_request_proto_msgTypes[2].OneofWrappers = []interface{}{
		(*PaymentMethod_Card)(nil),
		(*PaymentMethod_Wallet)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_schemas_external_v1_external_request_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   6,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_schemas_external_v1_external_request_proto_goTypes,
		DependencyIndexes: file_schemas_external_v1_external_request_proto_depIdxs,
		MessageInfos:      file_schemas_external_v1_external_request_proto_msgTypes,
	}.Build()
	File_schemas_external_v1_external_request_proto = out.File
	file_schemas_external_v1_external_request_proto_rawDesc = nil
	file_schemas_external_v1_external_request_proto_goTypes = nil
	file_schemas_external_v1_external_request_proto_depIdxs = nil
}
