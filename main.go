package main

import "C"
import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/avishrantssh/GoLicenseClassifier/classifier"
	"github.com/avishrantssh/GoLicenseClassifier/result"
)

// Default Threshold for Filtering the results
var defaultThreshold = 0.8

// Default Licenses Root Directory
var licensePath string

// Normalize Copyright Literals
var copyliteralRE = regexp.MustCompile(`&copy;|&copy|&#169;|&#xa9;|&#XA9;|u00A9|u00a9|\\xa9|\\XA9|\\251|©|\( C\)|(?i:\(c\))`)

// Regexp for Detecting Copyrights
var copyrightRE = regexp.MustCompile(`(?m)(?i:Copyright)\s+(?i:\(c\)\s+)?(?:\d{2,4})(?:[-,]\s*\d{2,4})*,?\s*(?i:by)?\s*(.*?(?i:\s+Inc\.)?)[.,-]?\s*(?i:All rights reserved\.?)?\s*$`)

// Removing in-text special code literals
var endliteralRE = regexp.MustCompile(`\\n|\\f|\\r|\\0`)

type FileContent struct {
	path string
	data []byte
	err  string
}

// CreateClassifier instantiates a classifier instance and loads base licenses
func CreateClassifier() (*classifier.Classifier, error) {
	c := classifier.NewClassifier(defaultThreshold)
	return c, c.LoadLicenses(licensePath)
}

func isDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return fileInfo.IsDir(), err
}

func FileReader(fileList []string, fileCh chan *FileContent) {
	defer close(fileCh)
	for _, path := range fileList {
		res := new(FileContent)
		res.path = path
		data, err := ioutil.ReadFile(path)
		if err != nil {
			res.err = err.Error()
		}
		res.data = data
		fileCh <- res
	}
}

//export FindMatch
func FindMatch(license *C.char, fpaths *C.char, outputPath *C.char, maxRoutines int) bool {
	PATH := C.GoString(fpaths)

	licensePath = C.GoString(license)

	// Channels, Mutex and WaitGroups
	var mutex sync.Mutex
	var wg sync.WaitGroup
	fileCh := make(chan *FileContent, 10)
	guard := make(chan struct{}, maxRoutines)

	paths := GetPaths(PATH)
	res := result.InitJSON(PATH, len(paths))
	wg.Add(len(paths))

	go FileReader(paths, fileCh)

	c, err := CreateClassifier()
	if err != nil {
		return false
	}

	for file := range fileCh {

		// Wait for guard channel to free-up
		guard <- struct{}{}
		go func(f *FileContent) {
			defer wg.Done()
			finfo := result.InitFile(f.path)

			if len(f.err) > 0 {
				finfo.Scan_Errors = append(finfo.Scan_Errors, f.err)
				res.AddFile(finfo)
				return
			}
			m := c.Match(f.data)
			for i := 0; i < m.Len(); i++ {
				finfo.Licenses = append(finfo.Licenses, result.License{
					Key:        m[i].Name,
					Confidence: m[i].Confidence,
					StartLine:  m[i].StartLine,
					EndLine:    m[i].EndLine,
					StartIndex: m[i].StartTokenIndex,
					EndIndex:   m[i].EndTokenIndex})

				finfo.Expression = append(finfo.Expression, m[i].Name)
			}
			cpInfo, tokens := CopyrightInfo(string(f.data))
			for i := 0; i < len(cpInfo); i++ {
				finfo.Copyrights = append(finfo.Copyrights, result.CpInfo{
					Expression: validate(cpInfo[i][0]),
					StartIndex: tokens[i][0],
					EndIndex:   tokens[i][1],
					Holder:     validate(cpInfo[i][1]),
				})
			}
			mutex.Lock()
			res.AddFile(finfo)
			mutex.Unlock()
			finfo = nil
			f = nil
			<-guard

		}(file)
	}

	wg.Wait()
	finishError := res.WriteJSON(C.GoString(outputPath))
	res = nil
	close(guard)
	return finishError == nil
}

// GetPaths crawls a given directory recursively and gives absolute path of all files
func GetPaths(fPath string) []string {
	dir, _ := isDirectory(fPath)
	fileList := []string{}
	if dir {
		filepath.Walk(fPath, func(path string, f os.FileInfo, err error) error {
			dir, _ := isDirectory(path)
			if dir {
				return nil
			}
			fileList = append(fileList, path)
			return nil
		})
	} else {
		fileList = []string{fPath}
	}
	return fileList
}

// CopyrightInfo finds a copyright notification, if it exists, and returns
// the copyright holder.
func CopyrightInfo(contents string) ([][]string, [][]int) {
	str := endliteralRE.ReplaceAllString(contents, "\n")
	normalizedString := copyliteralRE.ReplaceAllString(str, "(c)")

	matches := copyrightRE.FindAllStringSubmatch(normalizedString, -1)
	tokens := copyrightRE.FindAllStringSubmatchIndex(normalizedString, -1)

	var cpInfo [][]string
	for _, match := range matches {
		if len(match) == 2 {
			cpInfo = append(cpInfo, []string{strings.TrimSpace(match[0]), strings.TrimSpace(match[1])})
		}
	}
	return cpInfo, tokens
}

//export SetThreshold
func SetThreshold(thresh int) bool {
	if thresh < 0 || thresh > 100 {
		return false
	}
	defaultThreshold = float64(thresh) / 100.0
	return true
}

// Validate Strings before saving
func validate(test string) string {
	v := make([]rune, 0, len(test))
	for _, r := range test {
		if r == utf8.RuneError || r == '\x00' {
			break
		}
		v = append(v, r)
	}
	return string(v)
}

func main() {}
