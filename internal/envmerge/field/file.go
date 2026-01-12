package field

import "os"

type File struct {
	Dsc  *os.File
	Data map[string]string
}
