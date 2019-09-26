// Copyright 2019 The Morning Consult, LLC or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//         https://www.apache.org/licenses/LICENSE-2.0
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package main

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGetSecretPath(t *testing.T) {
	testSecret := "secret/docker/creds" // nolint: gosec

	t.Run("secret-in-env", func(t *testing.T) {
		old := os.Getenv(envSecretPath)
		defer os.Setenv(envSecretPath, old)
		os.Setenv(envSecretPath, testSecret)

		gotSecret, err := getSecretPath(nil)
		if err != nil {
			t.Fatal(err)
		}
		if gotSecret != testSecret {
			t.Errorf("Expected secret %s, got secret %s", testSecret, gotSecret)
		}
	})

	cases := []struct {
		name   string
		config map[string]interface{}
		err    string
		secret string
	}{
		{
			"no-secret-in-config",
			map[string]interface{}{},
			"The path to the secret where your Docker credentials are stored must be specified via either (1) the DCVL_SECRET environment variable or (2) the field 'auto_auth.config.secret' of the config file.", // nolint: lll
			"",
		},
		{
			"secret-is-not-string",
			map[string]interface{}{"secret": 12345},
			"field 'auto_auth.method.config.secret' could not be converted to string",
			"",
		},
		{
			"secret-is-empty",
			map[string]interface{}{"secret": ""},
			"field 'auto_auth.method.config.secret' is empty",
			"",
		},
		{
			"success",
			map[string]interface{}{"secret": testSecret},
			"",
			testSecret,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := os.Getenv(envSecretPath)
			defer os.Setenv(envSecretPath, old)
			os.Unsetenv(envSecretPath)

			gotSecret, err := getSecretPath(tc.config)
			if tc.err != "" {
				if err == nil {
					t.Fatal("expected an error but didn't receive one")
				}
				if err.Error() != tc.err {
					t.Fatalf("Results differ:\n%v", cmp.Diff(err.Error(), tc.err))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if gotSecret != tc.secret {
				t.Errorf("Expected secret %s, got secret %s", tc.secret, gotSecret)
			}
		})
	}
}

func TestNewLogWriter(t *testing.T) {
	noop := func() {}
	cases := []struct {
		name   string
		pre    func()
		config map[string]interface{}
		err    string
		post   func()
	}{
		{
			name: "log-dir-from-env",
			pre: func() {
				os.Setenv(envLogDir, "testdata")
			},
			err: "",
			post: func() {
				os.Unsetenv(envLogDir)
			},
		},
		{
			name: "log-dir-from-config",
			pre: func() {
				os.Unsetenv(envLogDir)
			},
			config: map[string]interface{}{"log_dir": "testdata"},
			err:    "",
			post:   noop,
		},
		{
			name:   "error-expanding-log-dir",
			pre:    noop,
			config: map[string]interface{}{"log_dir": "~asdgweq"},
			err:    "error expanding logging directory : cannot expand user-specific home dir",
			post:   noop,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.pre()
			defer tc.post()

			file, err := newLogWriter(tc.config)
			if tc.err != "" {
				if err == nil {
					t.Fatal("expected an error but didn't receive one")
				}
				if err.Error() != tc.err {
					t.Fatalf("Results differ:\n%v", cmp.Diff(err.Error(), tc.err))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			file.Close()
			filename := file.Name()
			if _, err = os.Stat(filename); err != nil {
				t.Fatal(err)
			}
			os.Remove(filename)
		})
	}
}
