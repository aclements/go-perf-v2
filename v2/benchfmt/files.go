// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package benchfmt

import "os"

// Files reads benchmark results from a sequence of input files.
//
// This reader adds a ".file" configuration key to the output Results
// containing the name of the file read in, exactly as it appears in
// the Paths list.
type Files struct {
	// Paths is the list of file names to read in.
	Paths []string

	// AllowStdin indicates that the path "-" should be treated as
	// stdin and if the file list is empty, it should be treated
	// as consisting of stdin.
	//
	// This is generally the desired behavior when the file list
	// comes from command-line flags.
	AllowStdin bool

	// pos is the position of the next file to read from in Paths
	// when the current file is exhausted.
	pos int

	reader  Reader
	path    string
	file    *os.File
	isStdin bool
	err     error
}

// Scan advances the reader to the next result in the sequence of
// files and returns true if a result was read. The caller should use
// the Result method to get the result. If an I/O error occurs, or
// this reaches the end of the file sequence, it returns false and the
// caller should use the Err method to check for errors.
func (f *Files) Scan() bool {
	if f.err != nil {
		return false
	}

	for {
		if f.file == nil {
			// Open the next file.
			var path string
			if f.AllowStdin && len(f.Paths) == 0 && f.pos == 0 {
				path = "-"
			} else if f.pos < len(f.Paths) {
				path = f.Paths[f.pos]
			} else {
				// We're out of files.
				return false
			}
			f.pos++
			f.path = path
			if f.AllowStdin && path == "-" {
				f.isStdin, f.file = true, os.Stdin
			} else {
				file, err := os.Open(path)
				if err != nil {
					f.err = err
					return false
				}
				f.isStdin, f.file = false, file
			}

			// Prepare the reader. Because ".file" is not
			// valid syntax for file configuration keys in
			// the file itself, there's no danger if it
			// being overwritten.
			f.reader.Reset(f.file, path, ".file", path)
		}

		// Try to get the next result.
		if f.reader.Scan() {
			return true
		}
		err := f.reader.Err()
		if err != nil {
			f.err = err
			break
		}
		// Just an EOF. Close this file and open the next.
		if !f.isStdin {
			f.file.Close()
		}
		f.file = nil
	}
	// We're out of files.
	return false
}

// Result returns the last result read, or an error if the result was
// malformed.
//
// Parse errors are non-fatal, so the caller can continue to call
// Scan.
//
// The caller should not retain the Result object, as it will be
// overwritten by the next call to Scan.
func (f *Files) Result() (*Result, error) {
	r, err := f.reader.Result()
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Err returns the first non-EOF I/O error that was encountered by the
// Files.
func (f *Files) Err() error {
	return f.err
}
