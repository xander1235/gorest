package enums

type HttpStatus int

const (
	Informational HttpStatus = 1
	Successful    HttpStatus = 2
	Redirection   HttpStatus = 3
	ClientError   HttpStatus = 4
	ServerError   HttpStatus = 5
)

func (status HttpStatus) Is1XXSeries() bool {
	return status/100 == Informational
}

func (status HttpStatus) Is2XXSeries() bool {
	return status/100 == Successful
}

func (status HttpStatus) Is3XXSeries() bool {
	return status/100 == Redirection
}

func (status HttpStatus) Is4XXSeries() bool {
	return status/100 == ClientError
}

func (status HttpStatus) Is5XXSeries() bool {
	return status/100 == ServerError
}

func (status HttpStatus) SeriesType() HttpStatus {
	return status / 100
}
