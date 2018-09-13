package test

import (
        "encoding/base64"
        "fmt"
        "net/http"
        "path"
        "strings"
        "testing"

        "github.com/hashicorp/vault/api"
        uuid "github.com/hashicorp/go-uuid"
        "github.com/phayes/freeport"
        "github.com/hashicorp/vault/helper/jsonutil"
)

type TestVaultServerOptions struct {
        SecretPath string
        Secret     map[string]interface{}
	Role       string
	PKCS7      string
}

type TestIAMAuthReqPayload struct {
        Role    string
        Method  string `json:"iam_http_request_method"`
        Url     string `json:"iam_request_url"`
        Body    string `json:"iam_request_body"`
        Headers string `json:"iam_request_headers"`
}

type TestEC2AuthReqPayload struct {
	Role  string
	PKCS7 string
}

// MakeMockVaultServerIAMAuth creates a mock Vault server which mimics two HTTP endpoints - 
// /v1/auth/aws/login and /v1/<secret_path>. The purpose of this mock Vault server
// is to test Vault's AWS IAM authentication endpoint without having to actually
// make a real sts:GetCallerIdentity request to AWS. The behavior of the mimicked
// endpoints is configured via the testVaultServerOptions parameter. The login
// endpoint will only return 200 when the JSON payload of an HTTP request for this 
// endpoint is properly structured and contains the expected data (see the IAM
// authentication information provided at 
// https://www.vaultproject.io/api/auth/aws/index.html#login) and when the 
// "role" field of the JSON payload matches the "role" field of the 
// testVaultServerOptions object passed to MakeMockVaultServerIAMAuth. The value of
// <secret_path> in the other endpoint is specified by the secretPath field of
// the testVaultServerOptions object. For example, if opts.secretPath == "secret/foo",
// your secret (specified via the "secret") field of the testVaultServerOptions
// object can be read via GET http://127.0.0.1:<port>/v1/secret/foo.
func MakeMockVaultServerIAMAuth(t *testing.T, opts *TestVaultServerOptions) *http.Server {
        port, err := freeport.GetFreePort()
        if err != nil {
                t.Fatal(err)
        }
        mux := http.NewServeMux()
        mux.HandleFunc("/v1/auth/aws/login", iamAuthHandler(t, opts.Role, port))
        if opts.SecretPath != "" {
                mux.HandleFunc(path.Join("/v1", opts.SecretPath), dockerSecretHandler(t, opts.Secret, port))
        }
        server := &http.Server{
                Addr:    fmt.Sprintf(":%d", port),
                Handler: mux,
        }
        return server
}

// MakeMockVaultServerEC2Auth creates a mock Vault server which mimics two HTTP 
// endpoints - /v1/auth/aws/login and /v1/<secret_path>. The purpose of this mock
// Vault server is to test Vault's AWS EC2 authentication endpoint without having
// to actually make a real call to AWS. The behavior of the mimicked endpoints is
// configured via the TestVaultServerOptions parameter. The login endpoint will
// only return 200 when the JSON payload of an HTTP request for this endpoint
// (1)is  properly structured, (2) contains the fields ("role" and "pkcs7"), 
// (3) the pkcs7 signature matches the value of the pkcs7 signature passed to 
// MakeMockVaultServerEC2Auth, and (4) the "role" field of the JSON payload matches
//  the "role" field of the TestVaultServerOptions object passed to 
// MakeMockVaultServerEC2Auth. This fourth condition mimics the behavior of Vault 
// in requiring a given role attempting to login via the AWS EC2 endpoint to have
// been explicitly configured to be able to do so. The value of <secret_path> in
// the other endpoint is specified by the secretPath field of the
// TestVaultServerOptions object. For example, if opts.secretPath == "secret/foo",
// your secret (specified via the "secret") field of the TestVaultServerOptions
// object can be read via GET http://127.0.0.1:<port>/v1/secret/foo.
func MakeMockVaultServerEC2Auth(t *testing.T, opts *TestVaultServerOptions) *http.Server {
        port, err := freeport.GetFreePort()
        if err != nil {
                t.Fatal(err)
        }
        mux := http.NewServeMux()
        mux.HandleFunc("/v1/auth/aws/login", ec2AuthHandler(t, opts.Role, opts.PKCS7, port))
        if opts.SecretPath != "" {
                mux.HandleFunc(path.Join("/v1", opts.SecretPath), dockerSecretHandler(t, opts.Secret, port))
        }
        server := &http.Server{
                Addr:    fmt.Sprintf(":%d", port),
                Handler: mux,
        }
        return server
}

func dockerSecretHandler(t *testing.T, secret map[string]interface{}, port int) http.HandlerFunc {
        return func(resp http.ResponseWriter, req *http.Request) {
                switch req.Method {
                case "GET":
                        prefix := fmt.Sprintf("[ GET http://127.0.0.1:%d/v1/auth/aws/login ]", port)
                        token := req.Header.Get("X-Vault-Token")
                        if token == "" {
                                t.Logf("%s request has no Vault token header\n", prefix)
                                http.Error(resp, "", 400)
                                return
                        }
                        if _, err := uuid.ParseUUID(token); err != nil {
                                t.Logf("%s unable to parse token %q: %v", prefix, token, err)
                                http.Error(resp, "", 500)
                                return
                        }

                        respData := &api.Secret{
                                Data: secret,
                        }

                        payload, err := jsonutil.EncodeJSON(respData)
                        if err != nil {
                                t.Logf("%s error encoding JSON response payload: %v\n", prefix, err)
                                http.Error(resp, "", 500)
                                return
                        }

                        resp.Header().Set("Content-Type", "application/json")
                        if _, err = resp.Write(payload); err != nil {
                                t.Logf("%s error writing response: %v\n", prefix, err)
                                http.Error(resp, "", 500)
                        }
                        return
                default:
                        http.Error(resp, "", 405)
                        return 
                }
        }
}

func iamAuthHandler(t *testing.T, role string, port int) http.HandlerFunc {
        return func(resp http.ResponseWriter, req *http.Request) {
                switch req.Method {
                case "POST", "PUT":
                        prefix := fmt.Sprintf("[ POST http://127.0.0.1:%d/v1/auth/aws/login ]", port)

                        var data = new(TestIAMAuthReqPayload)
                        if err := jsonutil.DecodeJSONFromReader(req.Body, data); err != nil {
                                t.Errorf("%s error unmarshaling response: %v\n", prefix, err)
                                http.Error(resp, "", 500)
                                return
                        }

                        if strings.ToLower(data.Role) != strings.ToLower(role) {
                                // t.Logf("%s role %q not configured for AWS authentication\n", prefix, role)
                                http.Error(resp, fmt.Sprintf("* entry for role %q not found", data.Role), 400)
                                return
                        }

                        if strings.ToLower(data.Method) != "post" {
                                // t.Logf("%s \"iam_http_request_method\" method field of JSON payload is not \"POST\"\n", prefix)
                                http.Error(resp, "", 400)
                                return
                        }

                        url, err := base64.StdEncoding.DecodeString(data.Url)
                        if err != nil {
                                // t.Logf("%s error base64 decoding \"iam_request_url\" field of JSON payload: %v\n", prefix, err)
                                http.Error(resp, "", 400)
                                return
                        }

                        if strings.TrimSuffix(string(url), "/") != "https://sts.amazonaws.com" {
                                // t.Logf("%s \"iam_request_url\" field of JSON payload is not \"https://sts.amazonaws.com/\"\n", prefix)
                                http.Error(resp, "", 400)
                                return
                        }

                        databody, err := base64.StdEncoding.DecodeString(data.Body)
                        if err != nil {
                                // t.Logf("%s error base64 decoding \"iam_request_body\" field of JSON payload: %v", prefix, err)
                                http.Error(resp, "", 400)
                                return
                        }
                        if string(databody) != "Action=GetCallerIdentity&Version=2011-06-15" {
                                // t.Logf("%s \"iam_request_body\" field of JSON payload is not \"Action=GetCallerIdentity&Version=2011-06-15\"\n", prefix)
                                http.Error(resp, "", 400)
                                return
                        }

                        headersBuf, err := base64.StdEncoding.DecodeString(data.Headers)
                        if err != nil {
                                // t.Logf("%s error base64 decoding \"iam_request_headers\" field of JSON payload: %v\n", prefix, err)
                                http.Error(resp, "", 400)
                                return
                        }
                        
                        var headers = make(map[string][]string)
                        if err = jsonutil.DecodeJSON(headersBuf, &headers); err != nil {
                                // t.Logf("%s error unmarshaling request headers: %v\n", prefix, err)
                                http.Error(resp, "", 400)
                                return
                        }

                        if _, ok := headers["Authorization"]; !ok {
                                // t.Logf("%s \"iam_request_headers\" field of JSON payload has no \"Authorization\" header\n", prefix)
                                http.Error(resp, "", 400)
                                return
                        }
                        // return the expected response with random uuid
                        token, err := uuid.GenerateUUID()
                        if err != nil {
                                t.Errorf("%s failed to create a random UUID: %v\n", prefix, err)
                                http.Error(resp, "", 500)
                                return
                        }

                        respData := &api.Secret{
                                Auth: &api.SecretAuth{
                                        ClientToken: token,
                                },
                        }

                        payload, err := jsonutil.EncodeJSON(respData)
                        if err != nil {
                                t.Errorf("%s error marshaling response payload: %v\n", prefix, err)
                                http.Error(resp, "", 500)
                                return
                        }

                        resp.Header().Set("Content-Type", "application/json")
                        resp.Write(payload)
                        return
                default:
                        http.Error(resp, "", 405)
                        return
                }
        }
}

func ec2AuthHandler(t *testing.T, role, pkcs7 string, port int) http.HandlerFunc {
        return func(resp http.ResponseWriter, req *http.Request) {
                switch req.Method {
                case "POST", "PUT":
                        prefix := fmt.Sprintf("[ POST http://127.0.0.1:%d/v1/auth/aws/login ]", port)

                        var data = new(TestEC2AuthReqPayload)
                        if err := jsonutil.DecodeJSONFromReader(req.Body, data); err != nil {
                                t.Errorf("%s error unmarshaling response: %v\n", prefix, err)
                                http.Error(resp, "", 500)
                                return
                        }

                        if strings.ToLower(data.Role) != strings.ToLower(role) {
                                http.Error(resp, fmt.Sprintf("* entry for role %q not found", data.Role), 400)
                                return
                        }

			if strings.Replace(pkcs7, "\n", "", -1) != data.PKCS7 {
				http.Error(resp, "* client nonce mismatch", 400)
				return
			}

                        // return the expected response with random uuid
                        token, err := uuid.GenerateUUID()
                        if err != nil {
                                t.Errorf("%s failed to create a random UUID: %v\n", prefix, err)
                                http.Error(resp, "", 500)
                                return
                        }

                        respData := &api.Secret{
                                Auth: &api.SecretAuth{
                                        ClientToken: token,
                                },
                        }

                        payload, err := jsonutil.EncodeJSON(respData)
                        if err != nil {
                                t.Errorf("%s error marshaling response payload: %v\n", prefix, err)
                                http.Error(resp, "", 500)
                                return
                        }

                        resp.Header().Set("Content-Type", "application/json")
                        resp.Write(payload)
                        return
                default:
                        http.Error(resp, "", 405)
                        return
                }
        }
}