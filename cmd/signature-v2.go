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
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Signature and API related constants.
const (
	signV2Algorithm = "AWS"
)

// AWS S3 Signature V2 calculation rule is give here:
// http://docs.aws.amazon.com/AmazonS3/latest/dev/RESTAuthentication.html#RESTAuthenticationStringToSign

// Whitelist resource list that will be used in query string for signature-V2 calculation.
// The list should be alphabetically sorted
var resourceList = []string{
	"acl",
	"delete",
	"lifecycle",
	"location",
	"logging",
	"notification",
	"partNumber",
	"policy",
	"requestPayment",
	"response-cache-control",
	"response-content-disposition",
	"response-content-encoding",
	"response-content-language",
	"response-content-type",
	"response-expires",
	"torrent",
	"uploadId",
	"uploads",
	"versionId",
	"versioning",
	"versions",
	"website",
}

func doesPolicySignatureV2Match(formValues http.Header) APIErrorCode {
	cred := serverConfig.GetCredential()
	accessKey := formValues.Get("AWSAccessKeyId")
	if accessKey != cred.AccessKey {
		return ErrInvalidAccessKeyID
	}
	policy := formValues.Get("Policy")
	signature := formValues.Get("Signature")
	if signature != calculateSignatureV2(policy, cred.SecretKey) {
		return ErrSignatureDoesNotMatch
	}
	return ErrNone
}

// doesPresignV2SignatureMatch - Verify query headers with presigned signature
//     - http://docs.aws.amazon.com/AmazonS3/latest/dev/RESTAuthentication.html#RESTAuthenticationQueryStringAuth
// returns ErrNone if matches. S3 errors otherwise.
func doesPresignV2SignatureMatch(r *http.Request) APIErrorCode {
	// Access credentials.
	cred := serverConfig.GetCredential()

	// r.RequestURI will have raw encoded URI as sent by the client.
	tokens := strings.SplitN(r.RequestURI, "?", 2)
	encodedResource := tokens[0]
	encodedQuery := ""
	if len(tokens) == 2 {
		encodedQuery = tokens[1]
	}

	queries := strings.Split(encodedQuery, "&")
	var filteredQueries []string
	var gotSignature string
	var expires string
	var accessKey string
	var err error
	for _, query := range queries {
		keyval := strings.Split(query, "=")
		switch keyval[0] {
		case "AWSAccessKeyId":
			accessKey, err = url.QueryUnescape(keyval[1])
		case "Signature":
			gotSignature, err = url.QueryUnescape(keyval[1])
		case "Expires":
			expires, err = url.QueryUnescape(keyval[1])
		default:
			unescapedQuery, qerr := url.QueryUnescape(query)
			if qerr == nil {
				filteredQueries = append(filteredQueries, unescapedQuery)
			} else {
				err = qerr
			}
		}
		// Check if the query unescaped properly.
		if err != nil {
			errorIf(err, "Unable to unescape query values", queries)
			return ErrInvalidQueryParams
		}
	}

	// Invalid access key.
	if accessKey == "" {
		return ErrInvalidQueryParams
	}

	// Validate if access key id same.
	if accessKey != cred.AccessKey {
		return ErrInvalidAccessKeyID
	}

	// Make sure the request has not expired.
	expiresInt, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return ErrMalformedExpires
	}

	// Check if the presigned URL has expired.
	if expiresInt < UTCNow().Unix() {
		return ErrExpiredPresignRequest
	}

	expectedSignature := preSignatureV2(r.Method, encodedResource, strings.Join(filteredQueries, "&"), r.Header, expires)
	if gotSignature != expectedSignature {
		return ErrSignatureDoesNotMatch
	}

	return ErrNone
}

// Authorization = "AWS" + " " + AWSAccessKeyId + ":" + Signature;
// Signature = Base64( HMAC-SHA1( YourSecretKey, UTF-8-Encoding-Of( StringToSign ) ) );
//
// StringToSign = HTTP-Verb + "\n" +
//  	Content-Md5 + "\n" +
//  	Content-Type + "\n" +
//  	Date + "\n" +
//  	CanonicalizedProtocolHeaders +
//  	CanonicalizedResource;
//
// CanonicalizedResource = [ "/" + Bucket ] +
//  	<HTTP-Request-URI, from the protocol name up to the query string> +
//  	[ subresource, if present. For example "?acl", "?location", "?logging", or "?torrent"];
//
// CanonicalizedProtocolHeaders = <described below>

// doesSignV2Match - Verify authorization header with calculated header in accordance with
//     - http://docs.aws.amazon.com/AmazonS3/latest/dev/auth-request-sig-v2.html
// returns true if matches, false otherwise. if error is not nil then it is always false

func validateV2AuthHeader(v2Auth string) APIErrorCode {
	if v2Auth == "" {
		return ErrAuthHeaderEmpty
	}
	// Verify if the header algorithm is supported or not.
	if !strings.HasPrefix(v2Auth, signV2Algorithm) {
		return ErrSignatureVersionNotSupported
	}

	// below is V2 Signed Auth header format, splitting on `space` (after the `AWS` string).
	// Authorization = "AWS" + " " + AWSAccessKeyId + ":" + Signature
	authFields := strings.Split(v2Auth, " ")
	if len(authFields) != 2 {
		return ErrMissingFields
	}

	// Then will be splitting on ":", this will seprate `AWSAccessKeyId` and `Signature` string.
	keySignFields := strings.Split(strings.TrimSpace(authFields[1]), ":")
	if len(keySignFields) != 2 {
		return ErrMissingFields
	}

	// Access credentials.
	cred := serverConfig.GetCredential()
	if keySignFields[0] != cred.AccessKey {
		return ErrInvalidAccessKeyID
	}

	return ErrNone
}

func doesSignV2Match(r *http.Request) APIErrorCode {
	v2Auth := r.Header.Get("Authorization")

	if apiError := validateV2AuthHeader(v2Auth); apiError != ErrNone {
		return apiError
	}

	// r.RequestURI will have raw encoded URI as sent by the client.
	tokens := strings.SplitN(r.RequestURI, "?", 2)
	encodedResource := tokens[0]
	encodedQuery := ""
	if len(tokens) == 2 {
		encodedQuery = tokens[1]
	}

	expectedAuth := signatureV2(r.Method, encodedResource, encodedQuery, r.Header)
	if v2Auth != expectedAuth {
		return ErrSignatureDoesNotMatch
	}

	return ErrNone
}

func calculateSignatureV2(stringToSign string, secret string) string {
	hm := hmac.New(sha1.New, []byte(secret))
	hm.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(hm.Sum(nil))
}

// Return signature-v2 for the presigned request.
func preSignatureV2(method string, encodedResource string, encodedQuery string, headers http.Header, expires string) string {
	cred := serverConfig.GetCredential()
	stringToSign := presignV2STS(method, encodedResource, encodedQuery, headers, expires)
	return calculateSignatureV2(stringToSign, cred.SecretKey)
}

// Return signature-v2 authrization header.
func signatureV2(method string, encodedResource string, encodedQuery string, headers http.Header) string {
	cred := serverConfig.GetCredential()
	stringToSign := signV2STS(method, encodedResource, encodedQuery, headers)
	signature := calculateSignatureV2(stringToSign, cred.SecretKey)
	return fmt.Sprintf("%s %s:%s", signV2Algorithm, cred.AccessKey, signature)
}

// Return canonical headers.
func canonicalizedAmzHeadersV2(headers http.Header) string {
	var keys []string
	keyval := make(map[string]string)
	for key := range headers {
		lkey := strings.ToLower(key)
		if !strings.HasPrefix(lkey, "x-amz-") {
			continue
		}
		keys = append(keys, lkey)
		keyval[lkey] = strings.Join(headers[key], ",")
	}
	sort.Strings(keys)
	var canonicalHeaders []string
	for _, key := range keys {
		canonicalHeaders = append(canonicalHeaders, key+":"+keyval[key])
	}
	return strings.Join(canonicalHeaders, "\n")
}

// Return canonical resource string.
func canonicalizedResourceV2(encodedPath string, encodedQuery string) string {
	queries := strings.Split(encodedQuery, "&")
	keyval := make(map[string]string)
	for _, query := range queries {
		key := query
		val := ""
		index := strings.Index(query, "=")
		if index != -1 {
			key = query[:index]
			val = query[index+1:]
		}
		keyval[key] = val
	}
	var canonicalQueries []string
	for _, key := range resourceList {
		val, ok := keyval[key]
		if !ok {
			continue
		}
		if val == "" {
			canonicalQueries = append(canonicalQueries, key)
			continue
		}
		// Resources values should be unescaped
		unescapedVal, err := url.QueryUnescape(val)
		if err != nil {
			errorIf(err, "Unable to unescape query value (query = `%s`, value = `%s`)", key, val)
			continue
		}
		canonicalQueries = append(canonicalQueries, key+"="+unescapedVal)
	}
	if len(canonicalQueries) == 0 {
		return encodedPath
	}
	// the queries will be already sorted as resourceList is sorted.
	return encodedPath + "?" + strings.Join(canonicalQueries, "&")
}

// Return string to sign for authz header calculation.
func signV2STS(method string, encodedResource string, encodedQuery string, headers http.Header) string {
	canonicalHeaders := canonicalizedAmzHeadersV2(headers)
	if len(canonicalHeaders) > 0 {
		canonicalHeaders += "\n"
	}

	// From the Amazon docs:
	//
	// StringToSign = HTTP-Verb + "\n" +
	// 	 Content-Md5 + "\n" +
	//	 Content-Type + "\n" +
	//	 Date + "\n" +
	//	 CanonicalizedProtocolHeaders +
	//	 CanonicalizedResource;
	stringToSign := strings.Join([]string{
		method,
		headers.Get("Content-MD5"),
		headers.Get("Content-Type"),
		headers.Get("Date"),
		canonicalHeaders,
	}, "\n") + canonicalizedResourceV2(encodedResource, encodedQuery)

	return stringToSign
}

// Return string to sign for pre-sign signature calculation.
func presignV2STS(method string, encodedResource string, encodedQuery string, headers http.Header, expires string) string {
	canonicalHeaders := canonicalizedAmzHeadersV2(headers)
	if len(canonicalHeaders) > 0 {
		canonicalHeaders += "\n"
	}

	// From the Amazon docs:
	//
	// StringToSign = HTTP-Verb + "\n" +
	// 	 Content-Md5 + "\n" +
	//	 Content-Type + "\n" +
	//	 Expires + "\n" +
	//	 CanonicalizedProtocolHeaders +
	//	 CanonicalizedResource;
	stringToSign := strings.Join([]string{
		method,
		headers.Get("Content-MD5"),
		headers.Get("Content-Type"),
		expires,
		canonicalHeaders,
	}, "\n") + canonicalizedResourceV2(encodedResource, encodedQuery)
	return stringToSign
}
