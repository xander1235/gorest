package enums

type RequestType int

const (
	RequestTypeType RequestType = iota
	Json
	Multipart
	FormUrlEncoded
)

func (s RequestType) Values() []string {
	return []string{"application/json", "multipart/form-data", "application/x-www-form-urlencoded"}
}

func (s RequestType) ToString() string {
	return s.Values()[s-1]
}

func (s RequestType) ValueOf(value string) RequestType {
	for i, g := range s.Values() {
		if g == value {
			return RequestType(i + 1)
		}
	}
	return -1
}
