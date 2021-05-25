package main

import "C"
import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/avishrantssh/GoLicenseClassifier/classifier"
)

var defaultThreshold = 0.8
var baseLicenses = "./classifier/licenses"

func New() (*classifier.Classifier, error) {
	c := classifier.NewClassifier(defaultThreshold)
	return c, c.LoadLicenses(baseLicenses)
}

//export FindMatch
func FindMatch(filepath *C.char) *C.char {
	var status []string
	patharr := GetPaths(C.GoString(filepath))
	ch := make(chan struct{})
	for _, path := range patharr {
		go func(path string) {
			b, err := ioutil.ReadFile(path)
			// File Not Found
			if err != nil {
				status = append(status, "E1,"+path)
			}

			data := []byte(string(b))

			c, err := New()
			// Internal Error in Initializing Classifier
			if err != nil {
				status = append(status, "E2,"+err.Error())
			}

			m := c.Match(data)
			var tmp string
			for i := 0; i < m.Len(); i++ {
				tmp += fmt.Sprintf("(%s,%f,%s,%d,%d),", m[i].Name, m[i].Confidence, m[i].MatchType, m[i].StartLine, m[i].EndLine)
			}

			// If No valid license is found
			if tmp == "" {
				status = append(status, "E3,"+path)
			} else {
				status = append(status, path+":"+tmp)
			}
			ch <- struct{}{}
		}(path)
	}
	for range patharr {
		<-ch
	}
	return C.CString(strings.Join(status, "\n"))
}

func GetPaths(filepath string) []string {
	return strings.SplitN(filepath, "\n", -1)
}

func main() {
	// 	str := `/home/avishrant/GitRepo/scancode.io/setup.py
	// /home/avishrant/GitRepo/scancode.io/CHANGELOG.rst
	// /home/avishrant/GitRepo/scancode.io/setup.cfg
	// /home/avishrant/GitRepo/scancode.io/docker.env
	// /home/avishrant/GitRepo/scancode.io/manage.py
	// /home/avishrant/GitRepo/scancode.io/NOTICE
	// /home/avishrant/GitRepo/scancode.io/LICENSE
	// /home/avishrant/GitRepo/scancode.io/docker-compose.yml
	// /home/avishrant/GitRepo/scancode.io/.env
	// /home/avishrant/GitRepo/scancode.io/Makefile
	// /home/avishrant/GitRepo/scancode.io/pyvenv.cfg
	// /home/avishrant/GitRepo/scancode.io/Dockerfile
	// /home/avishrant/GitRepo/scancode.io/.gitignore
	// /home/avishrant/GitRepo/scancode.io/README.rst
	// /home/avishrant/GitRepo/scancode.io/MANIFEST.in
	// /home/avishrant/GitRepo/scancode.io/scan.NOTICE`

	// fmt.Println(C.GoString(FindMatch(C.CString(str))))
}
