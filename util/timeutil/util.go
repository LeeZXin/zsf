package timeutil

import "time"

const (
	DefaultTimeFormat = "2006-01-02 15:04:05"
)

type JsonTime time.Time

func (t JsonTime) MarshalJSON() ([]byte, error) {
	ret := make([]byte, 0, len(DefaultTimeFormat)+2)
	ret = append(ret, '"')
	ret = time.Time(t).AppendFormat(ret, DefaultTimeFormat)
	ret = append(ret, '"')
	return ret, nil
}

func (t *JsonTime) UnmarshalJSON(data []byte) error {
	ret, err := time.Parse(`"`+DefaultTimeFormat+`"`, string(data))
	if err != nil {
		return err
	}
	*t = JsonTime(ret)
	return nil
}

func (t JsonTime) String() string {
	return time.Time(t).Format(DefaultTimeFormat)
}
