/*
 * Minio Cloud Storage, (C) 2016, 2017 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/minio/minio/pkg/lock"
)

func TestFSRenameFile(t *testing.T) {
	// create posix test setup
	_, path, err := newPosixTestSetup()
	if err != nil {
		t.Fatalf("Unable to create posix test setup, %s", err)
	}
	defer removeAll(path)

	if err = fsMkdir(pathJoin(path, "testvolume1")); err != nil {
		t.Fatal(err)
	}
	if err = fsRenameFile(pathJoin(path, "testvolume1"), pathJoin(path, "testvolume2")); err != nil {
		t.Fatal(err)
	}
	if err = fsRenameFile(pathJoin(path, "testvolume1"), pathJoin(path, "testvolume2")); errorCause(err) != errFileNotFound {
		t.Fatal(err)
	}
	if err = fsRenameFile(pathJoin(path, "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001"), pathJoin(path, "testvolume2")); errorCause(err) != errFileNameTooLong {
		t.Fatal("Unexpected error", err)
	}
	if err = fsRenameFile(pathJoin(path, "testvolume1"), pathJoin(path, "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001")); errorCause(err) != errFileNameTooLong {
		t.Fatal("Unexpected error", err)
	}
}

func TestFSStats(t *testing.T) {
	// create posix test setup
	_, path, err := newPosixTestSetup()
	if err != nil {
		t.Fatalf("Unable to create posix test setup, %s", err)
	}
	defer removeAll(path)

	// Setup test environment.

	if err = fsMkdir(""); errorCause(err) != errInvalidArgument {
		t.Fatal("Unexpected error", err)
	}

	if err = fsMkdir(pathJoin(path, "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001")); errorCause(err) != errFileNameTooLong {
		t.Fatal("Unexpected error", err)
	}

	if err = fsMkdir(pathJoin(path, "success-vol")); err != nil {
		t.Fatalf("Unable to create volume, %s", err)
	}

	var reader = bytes.NewReader([]byte("Hello, world"))
	if _, err = fsCreateFile(pathJoin(path, "success-vol", "success-file"), reader, nil, 0); err != nil {
		t.Fatalf("Unable to create file, %s", err)
	}
	// Seek back.
	reader.Seek(0, 0)

	if err = fsMkdir(pathJoin(path, "success-vol", "success-file")); errorCause(err) != errVolumeExists {
		t.Fatal("Unexpected error", err)
	}

	if _, err = fsCreateFile(pathJoin(path, "success-vol", "path/to/success-file"), reader, nil, 0); err != nil {
		t.Fatalf("Unable to create file, %s", err)
	}
	// Seek back.
	reader.Seek(0, 0)

	testCases := []struct {
		srcFSPath   string
		srcVol      string
		srcPath     string
		expectedErr error
	}{
		// Test case - 1.
		// Test case with valid inputs, expected to pass.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "success-file",
			expectedErr: nil,
		},
		// Test case - 2.
		// Test case with valid inputs, expected to pass.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "path/to/success-file",
			expectedErr: nil,
		},
		// Test case - 3.
		// Test case with non-existent file.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "nonexistent-file",
			expectedErr: errFileNotFound,
		},
		// Test case - 4.
		// Test case with non-existent file path.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "path/2/success-file",
			expectedErr: errFileNotFound,
		},
		// Test case - 5.
		// Test case with path being a directory.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "path",
			expectedErr: errFileAccessDenied,
		},
		// Test case - 6.
		// Test case with src path segment > 255.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			expectedErr: errFileNameTooLong,
		},
		// Test case - 7.
		// Test case validate only srcVol exists.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			expectedErr: nil,
		},
		// Test case - 8.
		// Test case validate only srcVol doesn't exist.
		{
			srcFSPath:   path,
			srcVol:      "success-vol-non-existent",
			expectedErr: errVolumeNotFound,
		},
		// Test case - 9.
		// Test case validate invalid argument.
		{
			expectedErr: errInvalidArgument,
		},
	}

	for i, testCase := range testCases {
		if testCase.srcPath != "" {
			if _, err := fsStatFile(pathJoin(testCase.srcFSPath, testCase.srcVol,
				testCase.srcPath)); errorCause(err) != testCase.expectedErr {
				t.Fatalf("TestPosix case %d: Expected: \"%s\", got: \"%s\"", i+1, testCase.expectedErr, err)
			}
		} else {
			if _, err := fsStatDir(pathJoin(testCase.srcFSPath, testCase.srcVol)); errorCause(err) != testCase.expectedErr {
				t.Fatalf("TestPosix case %d: Expected: \"%s\", got: \"%s\"", i+1, testCase.expectedErr, err)
			}
		}
	}
}

func TestFSCreateAndOpen(t *testing.T) {
	// Setup test environment.
	_, path, err := newPosixTestSetup()
	if err != nil {
		t.Fatalf("Unable to create posix test setup, %s", err)
	}
	defer removeAll(path)

	if err = fsMkdir(pathJoin(path, "success-vol")); err != nil {
		t.Fatalf("Unable to create directory, %s", err)
	}

	if _, err = fsCreateFile("", nil, nil, 0); errorCause(err) != errInvalidArgument {
		t.Fatal("Unexpected error", err)
	}

	if _, _, err = fsOpenFile("", -1); errorCause(err) != errInvalidArgument {
		t.Fatal("Unexpected error", err)
	}

	var reader = bytes.NewReader([]byte("Hello, world"))
	if _, err = fsCreateFile(pathJoin(path, "success-vol", "success-file"), reader, nil, 0); err != nil {
		t.Fatalf("Unable to create file, %s", err)
	}
	// Seek back.
	reader.Seek(0, 0)

	testCases := []struct {
		srcVol      string
		srcPath     string
		expectedErr error
	}{
		// Test case - 1.
		// Test case with segment of the volume name > 255.
		{
			srcVol:      "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			srcPath:     "success-file",
			expectedErr: errFileNameTooLong,
		},
		// Test case - 2.
		// Test case with src path segment > 255.
		{
			srcVol:      "success-vol",
			srcPath:     "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			expectedErr: errFileNameTooLong,
		},
	}

	for i, testCase := range testCases {
		_, err = fsCreateFile(pathJoin(path, testCase.srcVol, testCase.srcPath), reader, nil, 0)
		if errorCause(err) != testCase.expectedErr {
			t.Errorf("Test case %d: Expected: \"%s\", got: \"%s\"", i+1, testCase.expectedErr, err)
		}
		_, _, err = fsOpenFile(pathJoin(path, testCase.srcVol, testCase.srcPath), 0)
		if errorCause(err) != testCase.expectedErr {
			t.Errorf("Test case %d: Expected: \"%s\", got: \"%s\"", i+1, testCase.expectedErr, err)
		}
	}

	// Attempt to open a directory.
	if _, _, err = fsOpenFile(pathJoin(path), 0); errorCause(err) != errIsNotRegular {
		t.Fatal("Unexpected error", err)
	}
}

func TestFSDeletes(t *testing.T) {
	// create posix test setup
	_, path, err := newPosixTestSetup()
	if err != nil {
		t.Fatalf("Unable to create posix test setup, %s", err)
	}
	defer removeAll(path)

	// Setup test environment.
	if err = fsMkdir(pathJoin(path, "success-vol")); err != nil {
		t.Fatalf("Unable to create directory, %s", err)
	}

	var buf = make([]byte, 4096)
	var reader = bytes.NewReader([]byte("Hello, world"))
	if _, err = fsCreateFile(pathJoin(path, "success-vol", "success-file"), reader, buf, reader.Size()); err != nil {
		t.Fatalf("Unable to create file, %s", err)
	}
	// Seek back.
	reader.Seek(0, 0)

	testCases := []struct {
		srcVol      string
		srcPath     string
		expectedErr error
	}{
		// Test case - 1.
		// valid case with existing volume and file to delete.
		{
			srcVol:      "success-vol",
			srcPath:     "success-file",
			expectedErr: nil,
		},
		// Test case - 2.
		// The file was deleted in the last case, so DeleteFile should fail.
		{
			srcVol:      "success-vol",
			srcPath:     "success-file",
			expectedErr: errFileNotFound,
		},
		// Test case - 3.
		// Test case with segment of the volume name > 255.
		{
			srcVol:      "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			srcPath:     "success-file",
			expectedErr: errFileNameTooLong,
		},
		// Test case - 4.
		// Test case with src path segment > 255.
		{
			srcVol:      "success-vol",
			srcPath:     "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			expectedErr: errFileNameTooLong,
		},
	}

	for i, testCase := range testCases {
		if err = fsDeleteFile(path, pathJoin(path, testCase.srcVol, testCase.srcPath)); errorCause(err) != testCase.expectedErr {
			t.Errorf("Test case %d: Expected: \"%s\", got: \"%s\"", i+1, testCase.expectedErr, err)
		}
	}
}

// Tests fs removes.
func TestFSRemoves(t *testing.T) {
	// create posix test setup
	_, path, err := newPosixTestSetup()
	if err != nil {
		t.Fatalf("Unable to create posix test setup, %s", err)
	}
	defer removeAll(path)

	// Setup test environment.
	if err = fsMkdir(pathJoin(path, "success-vol")); err != nil {
		t.Fatalf("Unable to create directory, %s", err)
	}

	var reader = bytes.NewReader([]byte("Hello, world"))
	if _, err = fsCreateFile(pathJoin(path, "success-vol", "success-file"), reader, nil, 0); err != nil {
		t.Fatalf("Unable to create file, %s", err)
	}
	// Seek back.
	reader.Seek(0, 0)

	if _, err = fsCreateFile(pathJoin(path, "success-vol", "success-file-new"), reader, nil, 0); err != nil {
		t.Fatalf("Unable to create file, %s", err)
	}
	// Seek back.
	reader.Seek(0, 0)

	testCases := []struct {
		srcFSPath   string
		srcVol      string
		srcPath     string
		expectedErr error
	}{
		// Test case - 1.
		// valid case with existing volume and file to delete.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "success-file",
			expectedErr: nil,
		},
		// Test case - 2.
		// The file was deleted in the last case, so DeleteFile should fail.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "success-file",
			expectedErr: errFileNotFound,
		},
		// Test case - 3.
		// Test case with segment of the volume name > 255.
		{
			srcFSPath:   path,
			srcVol:      "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			srcPath:     "success-file",
			expectedErr: errFileNameTooLong,
		},
		// Test case - 4.
		// Test case with src path segment > 255.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			srcPath:     "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			expectedErr: errFileNameTooLong,
		},
		// Test case - 5.
		// Test case with src path empty.
		{
			srcFSPath:   path,
			srcVol:      "success-vol",
			expectedErr: errVolumeNotEmpty,
		},
		// Test case - 6.
		// Test case with src path empty.
		{
			srcFSPath:   path,
			srcVol:      "my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			expectedErr: errFileNameTooLong,
		},
		// Test case - 7.
		// Test case with src path empty.
		{
			srcFSPath:   path,
			srcVol:      "non-existent",
			expectedErr: errVolumeNotFound,
		},
		// Test case - 8.
		// Test case with src and volume path empty.
		{
			expectedErr: errInvalidArgument,
		},
	}

	for i, testCase := range testCases {
		if testCase.srcPath != "" {
			if err = fsRemoveFile(pathJoin(testCase.srcFSPath, testCase.srcVol, testCase.srcPath)); errorCause(err) != testCase.expectedErr {
				t.Errorf("Test case %d: Expected: \"%s\", got: \"%s\"", i+1, testCase.expectedErr, err)
			}
		} else {
			if err = fsRemoveDir(pathJoin(testCase.srcFSPath, testCase.srcVol, testCase.srcPath)); errorCause(err) != testCase.expectedErr {
				t.Error(err)
			}
		}
	}

	if err = fsRemoveAll(pathJoin(path, "success-vol")); err != nil {
		t.Fatal(err)
	}

	if err = fsRemoveAll(""); errorCause(err) != errInvalidArgument {
		t.Fatal(err)
	}

	if err = fsRemoveAll("my-obj-del-0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001"); errorCause(err) != errFileNameTooLong {
		t.Fatal(err)
	}
}

func TestFSRemoveMeta(t *testing.T) {
	// create posix test setup
	_, fsPath, err := newPosixTestSetup()
	if err != nil {
		t.Fatalf("Unable to create posix test setup, %s", err)
	}
	defer removeAll(fsPath)

	// Setup test environment.
	if err = fsMkdir(pathJoin(fsPath, "success-vol")); err != nil {
		t.Fatalf("Unable to create directory, %s", err)
	}

	filePath := pathJoin(fsPath, "success-vol", "success-file")

	var reader = bytes.NewReader([]byte("Hello, world"))
	if _, err = fsCreateFile(filePath, reader, nil, 0); err != nil {
		t.Fatalf("Unable to create file, %s", err)
	}

	rwPool := &fsIOPool{
		readersMap: make(map[string]*lock.RLockedFile),
	}

	if _, err := rwPool.Open(filePath); err != nil {
		t.Fatalf("Unable to lock file %s", filePath)
	}

	defer rwPool.Close(filePath)

	tmpDir, tmpErr := ioutil.TempDir(globalTestTmpDir, "minio-")
	if tmpErr != nil {
		t.Fatal(tmpErr)
	}

	if err := fsRemoveMeta(fsPath, filePath, tmpDir); err != nil {
		t.Fatalf("Unable to remove file, %s", err)
	}

	if _, err := osStat(preparePath(filePath)); !os.IsNotExist(err) {
		t.Fatalf("`%s` file found though it should have been deleted.", filePath)
	}

	if _, err := osStat(preparePath(path.Dir(filePath))); !os.IsNotExist(err) {
		t.Fatalf("`%s` parent directory found though it should have been deleted.", filePath)
	}
}
