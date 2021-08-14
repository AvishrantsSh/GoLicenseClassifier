package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/avishrantssh/GoLicenseClassifier/result"
	classifier "github.com/google/licenseclassifier/v2"
)

// Normalize Copyright Literals
var copyliteralRE = regexp.MustCompile(`&copy;|&copy|&#169;|&#xa9;|&#XA9;|u00A9|u00a9|\\xa9|\\XA9|\\251|©|\( C\)|(?i:\(c\))`)

// Regexp for Detecting Copyrights
var copyrightRE = regexp.MustCompile(`(?m)(?i:Copyright)\s+(?i:\(c\)\s+)?(?:\d{2,4}\s*)(?:[-,]\s*\d{2,4})*,?\s*(?i:by)?\s*(.*?(?i:\s+Inc\.)?)[.,-]?\s*(?i:All rights reserved\.?)?\s*$`)

// Removing in-text special code literals
var endliteralRE = regexp.MustCompile(`\\n|\\f|\\r|\\0`)

type FileContent struct {
	path string
	data []byte
	err  string
}

var gclassifier *classifier.Classifier

//export CreateClassifier
func CreateClassifier(license *C.char, defaultThreshold float64) {
	licensePath := C.GoString(license)
	gclassifier = classifier.NewClassifier(defaultThreshold)
	gclassifier.LoadLicenses(licensePath)
}

func isDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return fileInfo.IsDir(), err
}

func isLargeForScan(path string, size int) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return int(fileInfo.Size()/1000000) > size, nil
}

//export ScanFile
func ScanFile(fpaths *C.char, maxSize int, useBuffer bool) *C.char {
	if useBuffer {
		return BuffScanFile(fpaths, maxSize)
	}

	PATH := C.GoString(fpaths)
	finfo := result.InitFile(PATH)

	isLarge := false
	var error error

	if maxSize > 0 {
		isLarge, error = isLargeForScan(PATH, maxSize)
		if error != nil {
			finfo.Scan_Errors = append(finfo.Scan_Errors, error.Error())
		}
	}

	if isLarge {
		finfo.Scan_Errors = append(finfo.Scan_Errors, fmt.Sprint("File exceeds maximum size of ", maxSize))

	} else {
		data, fileErr := ioutil.ReadFile(PATH)
		if fileErr != nil {
			finfo.Scan_Errors = append(finfo.Scan_Errors, fileErr.Error())
		} else {
			match := gclassifier.Match(data)
			for i := 0; i < match.Len(); i++ {
				finfo.Licenses = append(finfo.Licenses, result.Licenses{
					Key:        match[i].Name,
					Score:      match[i].Confidence,
					StartLine:  match[i].StartLine,
					EndLine:    match[i].EndLine,
					StartIndex: match[i].StartTokenIndex,
					EndIndex:   match[i].EndTokenIndex})

				finfo.LicenseExpressions = append(finfo.LicenseExpressions, match[i].Name)
			}

			cpInfo, tokens := CopyrightInfo(string(data))
			for i := 0; i < len(cpInfo); i++ {
				finfo.Copyrights = append(finfo.Copyrights, result.Copyrights{
					Notification: validate(cpInfo[i][0]),
					// StartLine:    getLineNumber(data, tokens[i][0]),
					// EndLine:      getLineNumber(data, tokens[i][1]),
					StartIndex: tokens[i][0],
					EndIndex:   tokens[i][1],
				})

				finfo.Holders = append(finfo.Holders, result.Holder{
					Holder: validate(cpInfo[i][1]),
					// StartLine:  getLineNumber(data, tokens[i][2]),
					// EndLine:    getLineNumber(data, tokens[i][3]),
					StartIndex: tokens[i][2],
					EndIndex:   tokens[i][3],
				})
			}
			match = nil
			cpInfo = nil
			tokens = nil
		}
		data = nil
	}
	jString, jErr := finfo.GetJSONString()
	if jErr != nil {
		return C.CString("{\"error\":" + jErr.Error() + "}")
	}
	return C.CString(jString)
}

// BuffScanFile for using a buffered file scanning and analysis
func BuffScanFile(fpaths *C.char, bufferSize int) *C.char {
	PATH := C.GoString(fpaths)
	finfo := result.InitFile(PATH)

	file, err := os.Open(PATH)

	if err != nil {
		finfo.Scan_Errors = append(finfo.Scan_Errors, err.Error())
	} else {
		defer file.Close()
		buffer := make([]byte, bufferSize*1000000)
		for {
			bytesread, err := file.Read(buffer)
			if err != nil {
				if err != io.EOF {
					finfo.Scan_Errors = append(finfo.Scan_Errors, err.Error())
				}
				// If reached end of file
				break
			}

			match := gclassifier.Match(buffer[:bytesread])
			for i := 0; i < match.Len(); i++ {
				finfo.Licenses = append(finfo.Licenses, result.Licenses{
					Key:        match[i].Name,
					Score:      match[i].Confidence,
					StartLine:  match[i].StartLine,
					EndLine:    match[i].EndLine,
					StartIndex: match[i].StartTokenIndex,
					EndIndex:   match[i].EndTokenIndex})

				finfo.LicenseExpressions = append(finfo.LicenseExpressions, match[i].Name)
			}

			cpInfo, tokens := CopyrightInfo(string(buffer[:bytesread]))
			for i := 0; i < len(cpInfo); i++ {
				finfo.Copyrights = append(finfo.Copyrights, result.Copyrights{
					Notification: validate(cpInfo[i][0]),
					// StartLine:    getLineNumber(data, tokens[i][0]),
					// EndLine:      getLineNumber(data, tokens[i][1]),
					StartIndex: tokens[i][0],
					EndIndex:   tokens[i][1],
				})

				finfo.Holders = append(finfo.Holders, result.Holder{
					Holder: validate(cpInfo[i][1]),
					// StartLine:  getLineNumber(data, tokens[i][2]),
					// EndLine:    getLineNumber(data, tokens[i][3]),
					StartIndex: tokens[i][2],
					EndIndex:   tokens[i][3],
				})
			}
			match = nil
			cpInfo = nil
			tokens = nil
		}
	}
	jString, jErr := finfo.GetJSONString()
	if jErr != nil {
		return C.CString("{\"error\":" + jErr.Error() + "}")
	}
	return C.CString(jString)
}

// Go routine implementation of Scan File Function
func FindMatch(fpaths *C.char, outputPath *C.char, maxRoutines int) bool {
	PATH := C.GoString(fpaths)

	// Channels, and WaitGroups
	var wg sync.WaitGroup
	fileCh := make(chan *FileContent, 5)
	guard := make(chan struct{}, maxRoutines)

	paths := GetPaths(PATH)
	wg.Add(len(paths))

	go FileReader(paths, fileCh)

	for file := range fileCh {

		// Wait for guard channel to free-up
		guard <- struct{}{}
		go func(f *FileContent) {
			defer wg.Done()
			finfo := result.InitFile(f.path)

			m := gclassifier.Match(f.data)
			for i := 0; i < m.Len(); i++ {
				finfo.Licenses = append(finfo.Licenses, result.Licenses{
					Key:        m[i].Name,
					Score:      m[i].Confidence,
					StartLine:  m[i].StartLine,
					EndLine:    m[i].EndLine,
					StartIndex: m[i].StartTokenIndex,
					EndIndex:   m[i].EndTokenIndex})

				finfo.LicenseExpressions = append(finfo.LicenseExpressions, m[i].Name)
			}
			cpInfo, tokens := CopyrightInfo(string(f.data))
			for i := 0; i < len(cpInfo); i++ {
				finfo.Copyrights = append(finfo.Copyrights, result.Copyrights{
					Notification: validate(cpInfo[i][0]),
					StartIndex:   tokens[i][0],
					EndIndex:     tokens[i][1],
				})

				finfo.Holders = append(finfo.Holders, result.Holder{
					Holder: validate(cpInfo[i][1]),
					// StartLine:  getLineNumber(data, tokens[i][2]),
					// EndLine:    getLineNumber(data, tokens[i][3]),
					StartIndex: tokens[i][2],
					EndIndex:   tokens[i][3],
				})
			}
			finfo = nil
			f = nil
			<-guard

		}(file)
	}

	wg.Wait()
	close(guard)
	return true
}

// CopyrightInfo finds a copyright notification, if it exists, and returns
// the copyright holder.
func CopyrightInfo(contents string) ([][]string, [][]int) {
	str := endliteralRE.ReplaceAllString(contents, "\n")
	normalizedString := copyliteralRE.ReplaceAllString(str, "(c)")

	matches := copyrightRE.FindAllStringSubmatch(normalizedString, -1)
	tokens := copyrightRE.FindAllStringSubmatchIndex(normalizedString, -1)

	return matches, tokens
}

// Validate Strings before saving
func validate(test string) string {
	test = strings.TrimSpace(test)
	v := make([]rune, 0, len(test))
	for _, r := range test {
		if r == utf8.RuneError || r == '\x00' {
			break
		}
		v = append(v, r)
	}
	return string(v)
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

func main() {}
