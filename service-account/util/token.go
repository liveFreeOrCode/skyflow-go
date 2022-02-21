package util

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	fmt "fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/skyflowapi/skyflow-go/commonutils/errors"
	logger "github.com/skyflowapi/skyflow-go/commonutils/logwrapper"
	"github.com/skyflowapi/skyflow-go/commonutils/messages"
)

type ResponseToken struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:tokenType`
}

var tag = "GenerateBearerToken"

// Deprecated: Instaed use GenerateBearerToken
func GenerateToken(filePath string) (*ResponseToken, *errors.SkyflowError) {
	logger.Warn(fmt.Sprintf(messages.DEPRECATED_GENERATE_TOKEN_FUNCTION, tag))
	return GenerateBearerToken(filePath)
}

// GenerateBearerToken - Generates a Service Account Token from the given Service Account Credential file with a default timeout of 60minutes.
func GenerateBearerToken(filePath string) (*ResponseToken, *errors.SkyflowError) {
	var key map[string]interface{}

	logger.Info(fmt.Sprintf(messages.GENERATE_BEARER_TOKEN_TRIGGERED, tag))
	jsonFile, err := os.Open(filePath)
	if err != nil {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, fmt.Sprintf("Unable to open credentials - file %s", filePath)))
		return nil, errors.NewSkyflowErrorWrap(errors.InvalidInput, err, fmt.Sprintf("Unable to open credentials - file %s", filePath))
	}
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, fmt.Sprintf("Unable to read credentials - file %s", filePath)))
		return nil, errors.NewSkyflowErrorWrap(errors.InvalidInput, err, fmt.Sprintf("Unable to read credentials - file %s", filePath))
	}

	err = json.Unmarshal(byteValue, &key)
	if err != nil {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, fmt.Sprintf("Provided json file is in wrong format - file %s", filePath)))
		return nil, errors.NewSkyflowErrorWrap(errors.InvalidInput, err, fmt.Sprintf("Provided json file is in wrong format - file %s", filePath))
	}

	token, skyflowError := getSATokenFromCredsFile(key)
	if skyflowError != nil {
		return nil, skyflowError
	}
	return token, nil
}

func GenerateBearerTokenFromCreds(credentials string) (*ResponseToken, *errors.SkyflowError) {

	credsMap := make(map[string]interface{})
	logger.Info(fmt.Sprintf(messages.GENERATE_BEARER_TOKEN_TRIGGERED, tag))
	err := json.Unmarshal([]byte(credentials), &credsMap)
	if err != nil {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "credentials string is not a valid json string format"))
		return nil, errors.NewSkyflowErrorf(errors.InvalidInput, "credentials string is not a valid json string format")
	}

	token, skyflowError := getSATokenFromCredsFile(credsMap)
	if err != nil {
		return nil, skyflowError
	}
	return token, nil
}

// getSATokenFromCredsFile gets bearer token from service account endpoint
func getSATokenFromCredsFile(key map[string]interface{}) (*ResponseToken, *errors.SkyflowError) {
	pvtKey, skyflowError := getPrivateKeyFromPem(key["privateKey"].(string))
	if skyflowError != nil {
		return nil, skyflowError
	}

	clientID, ok := key["clientID"].(string)
	if !ok {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "Unable to read clientID"))
		return nil, errors.NewSkyflowErrorf(errors.InvalidInput, "Unable to read clientID")
	}
	keyID, ok := key["keyID"].(string)
	if !ok {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "Unable to read keyID"))
		return nil, errors.NewSkyflowErrorf(errors.InvalidInput, "Unable to read keyID")
	}
	tokenURI, ok := key["tokenURI"].(string)
	if !ok {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "Unable to read tokenURI"))
		return nil, errors.NewSkyflowErrorf(errors.InvalidInput, "Unable to read tokenURI")
	}

	signedUserJWT, skyflowError := getSignedUserToken(clientID, keyID, tokenURI, pvtKey)
	if skyflowError != nil {
		return nil, skyflowError
	}

	reqBody, err := json.Marshal(map[string]string{
		"grant_type": "urn:ietf:params:oauth:grant-type:jwt-bearer",
		"assertion":  signedUserJWT,
	})
	if err != nil {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "Unable to construct request payload"))
		return nil, errors.NewSkyflowErrorWrap(errors.InvalidInput, err, "Unable to construct request payload")
	}
	payload := strings.NewReader(string(reqBody))
	client := &http.Client{}
	req, err := http.NewRequest("POST", tokenURI, payload)
	if err != nil {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "Unable to create new request with tokenURI and payload"))
		return nil, errors.NewSkyflowErrorWrap(errors.InvalidInput, err, "Unable to create new request with tokenURI and payload")
	}
	req.Header.Add("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		logger.Error(fmt.Sprintf(messages.SERVER_ERROR, tag, "Internal server error"))
		return nil, errors.NewSkyflowErrorWrap(errors.Server, err, "Internal server error")
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		logger.Error(fmt.Sprintf(messages.SERVER_ERROR, tag, "Unable to read response payload"))
		return nil, errors.NewSkyflowErrorWrap(errors.Server, err, "Unable to read response payload")
	}

	if res.StatusCode != 200 {
		logger.Error(fmt.Sprintf(messages.SERVER_ERROR, tag, fmt.Sprintf("%v", string(body))))
		return nil, errors.NewSkyflowErrorWrap(errors.Server,
			fmt.Errorf("%v", string(body)),
			"Error Occured")
	}

	if len(body) == 0 {
		logger.Error(fmt.Sprintf(messages.SERVER_ERROR, tag, "Empty body"))
		return nil, errors.NewSkyflowError(errors.Server, "Empty body")
	}

	var responseToken ResponseToken
	json.Unmarshal([]byte(body), &responseToken)
	logger.Info(fmt.Sprintf(messages.GENERATE_BEARER_TOKEN_SUCCESS, tag))
	return &responseToken, nil
}

func getPrivateKeyFromPem(pemKey string) (*rsa.PrivateKey, *errors.SkyflowError) {
	var err error
	privPem, _ := pem.Decode([]byte(pemKey))
	if privPem == nil {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "Unable to decode the RSA private PEM"))
		return nil, errors.NewSkyflowErrorWrap(errors.InvalidInput, err, "Unable to decode the RSA private PEM")
	}

	if privPem.Type != "PRIVATE KEY" {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, fmt.Sprintf("RSA private key is of the wrong type Pem Type: %s", privPem.Type)))
		return nil, errors.NewSkyflowErrorf(errors.InvalidInput, "RSA private key is of the wrong type Pem Type: %s", privPem.Type)
	}

	var parsedKey interface{}
	if parsedKey, err = x509.ParsePKCS1PrivateKey(privPem.Bytes); err != nil {
		if parsedKey, err = x509.ParsePKCS8PrivateKey(privPem.Bytes); err != nil {
			logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "Unable to parse RSA private key"))
			return nil, errors.NewSkyflowErrorWrap(errors.InvalidInput, err, "Unable to parse RSA private key")
		}
	}

	var privateKey *rsa.PrivateKey
	var ok bool
	privateKey, ok = parsedKey.(*rsa.PrivateKey)
	if !ok {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "Unable to retrieve RSA private key"))
		return nil, errors.NewSkyflowErrorf(errors.InvalidInput, "Unable to retrieve RSA private key")
	}
	return privateKey, nil
}

func getSignedUserToken(clientID, keyID, tokenURI string, pvtKey *rsa.PrivateKey) (string, *errors.SkyflowError) {

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": clientID,
		"key": keyID,
		"aud": tokenURI,
		"sub": clientID,
		"exp": time.Now().Add(60 * time.Minute).Unix(),
	})

	var err error
	signedToken, err := token.SignedString(pvtKey)
	if err != nil {
		logger.Error(fmt.Sprintf(messages.INVALID_INPUT, tag, "unable to parse jwt payload"))
		return "", errors.NewSkyflowErrorWrap(errors.InvalidInput, err, "unable to parse jwt payload")
	}
	return signedToken, nil
}
