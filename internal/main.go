package internal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
)

// Структура запроса API Gateway v1
type APIGatewayRequest struct {
	OperationID string `json:"operationId"`
	Resource    string `json:"resource"`

	HTTPMethod string `json:"httpMethod"`

	Path           string            `json:"path"`
	PathParameters map[string]string `json:"pathParameters"`

	Headers           map[string]string   `json:"headers"`
	MultiValueHeaders map[string][]string `json:"multiValueHeaders"`

	QueryStringParameters           map[string]string   `json:"queryStringParameters"`
	MultiValueQueryStringParameters map[string][]string `json:"multiValueQueryStringParameters"`

	Parameters           map[string]string   `json:"parameters"`
	MultiValueParameters map[string][]string `json:"multiValueParameters"`

	Body            string `json:"body"`
	IsBase64Encoded bool   `json:"isBase64Encoded,omitempty"`

	RequestContext interface{} `json:"requestContext"`
}

// Структура ответа API Gateway v1
type APIGatewayResponse struct {
	StatusCode        int                 `json:"statusCode"`
	Headers           map[string]string   `json:"headers"`
	MultiValueHeaders map[string][]string `json:"multiValueHeaders"`
	Body              string              `json:"body"`
	IsBase64Encoded   bool                `json:"isBase64Encoded,omitempty"`
}

type Request struct {
	UpdateID int64 `json:"update_id"`
	Message  struct {
		ID   int64 `json:"message_id"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Photo []struct {
			ID       string `json:"file_id"`
			UniqueID string `json:"file_unique_id"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
		} `json:"photo,omitempty"`
	} `json:"message"`
}

type GetFilePathResp struct {
	Result struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}

type OCRRequest struct {
	MimeType      string   `json:"mimeType"`
	LanguageCodes []string `json:"languageCodes"`
	Model         string   `json:"model"`
	Content       string   `json:"content"`
}

type OCRResp struct {
	Result struct {
		TextAnnotation struct {
			FullText string `json:"fullText"`
		} `json:"textAnnotation"`
	} `json:"result"`
}

type SendMsgReq struct {
	ChatId           int64  `json:"chat_id"`
	Text             string `json:"text"`
	ReplyToMessageId int64  `json:"reply_to_message_id"`
	ParseMode        string `json:"parse_mode,omitempty"`
}

type YaGPTRequest struct {
	ModelUri          string                 `json:"modelUri"`
	CompletionOptions YaGPTRequestOptions    `json:"completionOptions"`
	Messages          []YaGPTRequestMessages `json:"messages"`
}

type YaGPTRequestOptions struct {
	Stream      bool    `json:"stream"`
	Temperature float64 `json:"temperature"`
	MaxTokens   string  `json:"maxTokens"`
}

type YaGPTRequestMessages struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type YaGPTResponse struct {
	Result struct {
		Alternatives []struct {
			Message struct {
				Role string `json:"role"`
				Text string `json:"text"`
			} `json:"message"`
			Status string `json:"status"`
		} `json:"alternatives"`
		Usage struct {
			InputTextTokens  string `json:"inputTextTokens"`
			CompletionTokens string `json:"completionTokens"`
			TotalTokens      string `json:"totalTokens"`
		} `json:"usage"`
		ModelVersion string `json:"modelVersion"`
	} `json:"result"`
}

const (
	getFilePathURLPattern  = "https://api.telegram.org/bot%s/getFile?file_id=%s"
	sendMsgURLPattern      = "https://api.telegram.org/bot%s/sendMessage"
	downloadFileURLPattern = "https://api.telegram.org/file/bot%s"
	localPath              = "/function/storage/images"
	ocrURL                 = "https://ocr.api.cloud.yandex.net/ocr/v1/recognizeText"
	catalog                = "b1g163vdicpkeevao9ga"
	yaGPTURL               = "https://llm.api.cloud.yandex.net/foundationModels/v1/completion"
	maxMessageLen          = 4096
)

func Handler(ctx context.Context, event *APIGatewayRequest) (*APIGatewayResponse, error) {
	token := os.Getenv("TG_API_KEY")
	req := &Request{}

	// Поле event.Body запроса преобразуется в объект типа Request для получения переданного имени
	if err := json.Unmarshal([]byte(event.Body), &req); err != nil {
		return nil, fmt.Errorf("an error has occurred when parsing body: %w", err)
	}

	if len(req.Message.Photo) == 0 {
		if err := sendReply(
			req.Message.Chat.ID,
			"Пришлите мне фотографию вашего экзаменационного билета и я его попытаюсь решить его",
			req.Message.ID,
		); err != nil {
			return nil, fmt.Errorf("failed to send reply: %w", err)
		}

		return &APIGatewayResponse{
			StatusCode: 200,
		}, nil
	}

	fileID := req.Message.Photo[len(req.Message.Photo)-1].ID

	get, err := http.Get(fmt.Sprintf(getFilePathURLPattern, token, fileID))
	if err != nil {
		return nil, fmt.Errorf("failed to get file path: %w", err)
	}

	defer get.Body.Close()
	body, err := io.ReadAll(get.Body)

	filePathResp := &GetFilePathResp{}
	if err := json.Unmarshal(body, &filePathResp); err != nil {
		return nil, fmt.Errorf("failed to parse resp body: %w", err)
	}

	downloadPath := path.Join(fmt.Sprintf(downloadFileURLPattern, token), filePathResp.Result.FilePath)

	filePath := path.Join(localPath, fileID) + ".jpg"

	if err := downloadFile(filePath, downloadPath); err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}

	ocrText, err := proceedOCR(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to recognize text: %w", err)
	}

	fmt.Printf("prompt: %v\n", ocrText)

	promptResult, err := doPrompt(ocrText)
	if err != nil {
		return nil, fmt.Errorf("failed to proceed prompt: %w", err)
	}

	if err := sendReply(req.Message.Chat.ID, promptResult, req.Message.ID); err != nil {
		return nil, fmt.Errorf("failed to send reply: %w", err)
	}

	return &APIGatewayResponse{
		StatusCode: 200,
	}, nil
}

func downloadFile(filepath string, url string) (err error) {
	cmd := exec.Command("curl", url, "--output", filepath)

	return cmd.Run()
}

func proceedOCR(path string) (string, error) {
	apiToken := os.Getenv("OCR_API_KEY")
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}

	// Прочитайте содержимое файла.
	reader := bufio.NewReader(f)
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	// Получите содержимое файла в формате Base64.
	base64Img := base64.StdEncoding.EncodeToString(content)

	ocrBody := OCRRequest{
		MimeType:      "JPEG",
		LanguageCodes: []string{"ru"},
		Model:         "page",
		Content:       base64Img,
	}

	ocrBodyBytes, err := json.Marshal(ocrBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", ocrURL, bytes.NewReader(ocrBodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Api-Key "+apiToken)
	req.Header.Set("x-data-logging-enabled", "true")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	ocrResp := &OCRResp{}

	if err := json.Unmarshal(body, ocrResp); err != nil {
		return "", err
	}

	return ocrResp.Result.TextAnnotation.FullText, nil
}

func sendReply(chatID int64, text string, replyToMsgID int64) error {
	token := os.Getenv("TG_API_KEY")

	texts := make([]string, 0)
	if len(text) > maxMessageLen {
		texts = append(texts, text[:maxMessageLen])
		texts = append(texts, text[maxMessageLen:])
	} else {
		texts = append(texts, text)
	}

	for i := 0; i < len(texts); i++ {
		sendReplyBody := &SendMsgReq{
			ChatId:           chatID,
			Text:             texts[i],
			ReplyToMessageId: replyToMsgID,
			ParseMode:        "Markdown",
		}

		sendReplyBodyBytes, err := json.Marshal(sendReplyBody)
		if err != nil {
			return err
		}

		resp, err := http.Post(
			fmt.Sprintf(sendMsgURLPattern, token),
			"application/json",
			bytes.NewReader(sendReplyBodyBytes))
		if err != nil {
			return err
		}

		if resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			return errors.New("failed to send reply tg message: " + resp.Status + " " + string(body))
		}

	}

	return nil
}

func doPrompt(prompt string) (string, error) {
	request := &YaGPTRequest{
		ModelUri: "gpt://" + catalog + "/yandexgpt-lite",
		CompletionOptions: YaGPTRequestOptions{
			Stream:      false,
			Temperature: 0.3,
			MaxTokens:   "2000",
		},
		Messages: []YaGPTRequestMessages{
			{
				Role: "system",
				Text: "Ты преподаватель по компьютерным наукам. Ответь на следующие билеты на экзамене",
			},
			{
				Role: "user",
				Text: prompt,
			},
		},
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", yaGPTURL, bytes.NewReader(requestBytes))
	if err != nil {
		return "", err
	}

	apiToken := os.Getenv("YAGPT_API_KEY")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Api-Key "+apiToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", errors.New("yagpt request failed with status: " + resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	yaGPTResp := &YaGPTResponse{}

	if err := json.Unmarshal(body, yaGPTResp); err != nil {
		return "", err
	}

	if len(yaGPTResp.Result.Alternatives) == 0 {
		return "no answer :(", nil
	}

	return yaGPTResp.Result.Alternatives[0].Message.Text, nil
}
