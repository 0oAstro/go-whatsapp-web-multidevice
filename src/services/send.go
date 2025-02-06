package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/app"
	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/internal/rest/helpers"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/validations"
	"github.com/disintegration/imaging"
	fiberUtils "github.com/gofiber/fiber/v2/utils"
	"github.com/sirupsen/logrus"
	"github.com/valyala/fasthttp"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type serviceSend struct {
	WaCli      *whatsmeow.Client
	appService app.IAppService
}

func NewSendService(waCli *whatsmeow.Client, appService app.IAppService) domainSend.ISendService {
	return &serviceSend{
		WaCli:      waCli,
		appService: appService,
	}
}

// wrapSendMessage wraps the message sending process with message ID saving
func (service serviceSend) wrapSendMessage(ctx context.Context, recipient types.JID, msg *waE2E.Message, _ string) (whatsmeow.SendResponse, error) {
	ts, err := service.WaCli.SendMessage(ctx, recipient, msg)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	// Uncomment to enable message storage
	// utils.RecordMessage(ts.ID, service.WaCli.Store.ID.String(), content)

	return ts, nil
}

func downloadFileFromURL(url, path string) error {
	resp, err := http.Get(url)
	if (err != nil) {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(path)
	if (err != nil) {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func normalizeFileName(name string) string {
	decoded, err := url.QueryUnescape(filepath.Base(name))
	if err != nil {
		decoded = filepath.Base(name)
	}
	cleaned := filepath.Clean(decoded)
	cleaned = strings.ReplaceAll(cleaned, " ", "_")
	fmt.Printf("Cleaned filename: %s\n", cleaned)
	return cleaned
}

func (service serviceSend) SendText(ctx context.Context, request domainSend.MessageRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendMessage(ctx, request)
	if err != nil {
		return response, err
	}
	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	// Send message
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(request.Message),
		},
	}

	parsedMentions := service.getMentionFromText(ctx, request.Message)
	if len(parsedMentions) > 0 {
		msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
			MentionedJID: parsedMentions,
		}
	}

	// Reply message
	if request.ReplyMessageID != nil && *request.ReplyMessageID != "" {
		record, err := utils.FindRecordFromStorage(*request.ReplyMessageID)
		if err == nil { // Only set reply context if we found the message ID
			msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
				Text: proto.String(request.Message),
				ContextInfo: &waE2E.ContextInfo{
					StanzaID:    request.ReplyMessageID,
					Participant: proto.String(record.JID),
					QuotedMessage: &waE2E.Message{
						Conversation: proto.String(record.MessageContent),
					},
				},
			}

			if len(parsedMentions) > 0 {
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = parsedMentions
			}
		} else {
			logrus.Warnf("Reply message ID %s not found in storage, continuing without reply context", *request.ReplyMessageID)
		}
	}

	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, msg, request.Message)
	if err != nil {
		return response, err
	}

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Message sent to %s (server timestamp: %s)", request.Phone, ts.Timestamp.String())
	return response, nil
}

func (service serviceSend) SendImage(ctx context.Context, request domainSend.ImageRequest) (response domainSend.GenericResponse, err error) {
	logrus.WithFields(logrus.Fields{
		"phone": request.Phone,
		"url":   request.ImageUrl,
		"file":  request.Image != nil,
	}).Debug("SendImage request received")

	err = validations.ValidateSendImage(ctx, request)
	if err != nil {
		return response, err
	}

	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	var (
		oriImagePath   string
		imagePath      string
		imageThumbnail string
		deletedItems   []string
		filename      string
	)

	// Generate unique filename
	filename = fmt.Sprintf("image-%s%s", fiberUtils.UUIDv4(), ".jpg")
	oriImagePath = fmt.Sprintf("%s/%s", config.PathSendItems, filename)

	// Handle image from URL or file
	if request.ImageUrl != "" {
		logrus.WithField("url", request.ImageUrl).Debug("Downloading image from URL")
		err = downloadFileFromURL(request.ImageUrl, oriImagePath)
		if err != nil {
			return response, fmt.Errorf("failed to download image: %v", err)
		}
		deletedItems = append(deletedItems, oriImagePath)
	} else if request.Image != nil {
		logrus.WithField("filename", request.Image.Filename).Debug("Saving uploaded image")
		err = fasthttp.SaveMultipartFile(request.Image, oriImagePath)
		if err != nil {
			return response, fmt.Errorf("failed to save image: %v", err)
		}
		deletedItems = append(deletedItems, oriImagePath)
	} else {
		return response, pkgError.ValidationError("either ImageUrl or Image must be provided")
	}

	// Generate thumbnail
	imageThumbnail = fmt.Sprintf("%s/thumb-%s", config.PathSendItems, filename)
	srcImage, err := imaging.Open(oriImagePath)
	if err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("failed to open image %v", err))
	}
	resizedImage := imaging.Resize(srcImage, 100, 0, imaging.Lanczos)
	if err = imaging.Save(resizedImage, imageThumbnail); err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("failed to save thumbnail %v", err))
	}
	deletedItems = append(deletedItems, imageThumbnail)

	// Handle compression if needed
	if request.Compress {
		logrus.Debug("Compressing image")
		newImagePath := fmt.Sprintf("%s/compressed-%s", config.PathSendItems, filename)
		newImage := imaging.Resize(srcImage, 600, 0, imaging.Lanczos)
		if err = imaging.Save(newImage, newImagePath); err != nil {
			return response, pkgError.InternalServerError(fmt.Sprintf("failed to save compressed image %v", err))
		}
		deletedItems = append(deletedItems, newImagePath)
		imagePath = newImagePath
	} else {
		imagePath = oriImagePath
	}

	// Send to WhatsApp
	logrus.Debug("Uploading to WhatsApp")
	dataWaImage, err := os.ReadFile(imagePath)
	if err != nil {
		return response, err
	}
	uploadedImage, err := service.uploadMedia(ctx, whatsmeow.MediaImage, dataWaImage, dataWaRecipient)
	if err != nil {
		return response, fmt.Errorf("failed to upload image: %v", err)
	}

	dataWaThumbnail, err := os.ReadFile(imageThumbnail)
	if err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("failed to read thumbnail %v", err))
	}

	// Prepare and send message
	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		JPEGThumbnail: dataWaThumbnail,
		Caption:       proto.String(request.Caption),
		URL:           proto.String(uploadedImage.URL),
		DirectPath:    proto.String(uploadedImage.DirectPath),
		MediaKey:      uploadedImage.MediaKey,
		Mimetype:      proto.String(http.DetectContentType(dataWaImage)),
		FileEncSHA256: uploadedImage.FileEncSHA256,
		FileSHA256:    uploadedImage.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(dataWaImage))),
		ViewOnce:      proto.Bool(request.ViewOnce),
	}}

	caption := ""
	if request.Caption != "" {
		caption = request.Caption
	}
	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, msg, caption)
	go func() {
		errDelete := utils.RemoveFile(0, deletedItems...)
		if errDelete != nil {
			fmt.Println("error when deleting picture: ", errDelete)
		}
	}()
	if err != nil {
		return response, err
	}

	// Cleanup files
	// go func() {
	// 	if err := utils.RemoveFile(0, deletedItems...); err != nil {
	// 		logrus.WithError(err).Error("Failed to cleanup files")
	// 	}
	// }()

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Image sent to %s (server timestamp: %s)", request.Phone, ts.Timestamp)
	return response, nil
}

func (service serviceSend) SendFile(ctx context.Context, request domainSend.FileRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendFile(ctx, request)
	if err != nil {
		return response, err
	}
	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	var (
		filePath     string
		deletedItems []string
		filename     string
	)

	generateUUID := fiberUtils.UUIDv4()

	// Handle file from URL or uploaded file
	if request.FileUrl != "" {
		filename = normalizeFileName(request.FileUrl)
		filePath = fmt.Sprintf("%s/%s", config.PathSendItems, filename)
		err = downloadFileFromURL(request.FileUrl, filePath)
		if err != nil {
			return response, fmt.Errorf("failed to download file: %v", err)
		} else {
			fmt.Printf("Downloaded file from URL: %s", request.FileUrl)
		}
		deletedItems = append(deletedItems, filePath)
	} else if request.File != nil {
		if request.File.Filename == "" {
			return response, pkgError.ValidationError("file: filename cannot be empty")
		}
		filename = normalizeFileName(request.File.Filename)
		filePath = fmt.Sprintf("%s/%s-%s", config.PathSendItems, generateUUID, filename)
		err = fasthttp.SaveMultipartFile(request.File, filePath)
		if err != nil {
			return response, pkgError.InternalServerError(fmt.Sprintf("failed to store file in server %v", err))
		}
		deletedItems = append(deletedItems, filePath)
	} else {
		return response, pkgError.ValidationError("either FileUrl or File must be provided")
	}

	// Read file data
	dataWaFile, err := os.ReadFile(filePath)
	if err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("failed to read file %v", err))
	}

	// Upload to WhatsApp server
	fileMimeType := http.DetectContentType(dataWaFile)
	uploadedFile, err := service.uploadMedia(ctx, whatsmeow.MediaDocument, dataWaFile, dataWaRecipient)
	if err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("failed to upload file: %v", err))
	}

	// Prepare and send message
	msg := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		URL:           proto.String(uploadedFile.URL),
		Mimetype:      proto.String(fileMimeType),
		Title:         proto.String(filename),
		FileSHA256:    uploadedFile.FileSHA256,
		FileLength:    proto.Uint64(uploadedFile.FileLength),
		MediaKey:      uploadedFile.MediaKey,
		FileName:      proto.String(filename),
		FileEncSHA256: uploadedFile.FileEncSHA256,
		DirectPath:    proto.String(uploadedFile.DirectPath),
		Caption:       proto.String(request.Caption),
	}}
	caption := ""
	if request.Caption != "" {
		caption = request.Caption
	}
	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, msg, caption)
	if err != nil {
		return response, err
	}

	// Cleanup files
	go func() {
		if err := utils.RemoveFile(1, deletedItems...); err != nil {
			logrus.WithError(err).Error("Failed to cleanup files")
		}
	}()

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Document sent to %s (server timestamp: %s)", request.Phone, ts.Timestamp.String())
	return response, nil
}

func (service serviceSend) SendVideo(ctx context.Context, request domainSend.VideoRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendVideo(ctx, request)
	if err != nil {
		return response, err
	}
	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	var (
		oriVideoPath   string
		videoPath      string
		videoThumbnail string
		deletedItems   []string
	)

	generateUUID := fiberUtils.UUIDv4()
	// Save video to server
	if request.VideoUrl != "" {
		oriVideoPath = fmt.Sprintf("%s/url-video-%s.mp4", config.PathSendItems, generateUUID)
		err = downloadFileFromURL(request.VideoUrl, oriVideoPath)
		if err != nil {
			return response, err
		}
	} else if request.Video != nil {
		oriVideoPath = fmt.Sprintf("%s/%s", config.PathSendItems, generateUUID+request.Video.Filename)
		err = fasthttp.SaveMultipartFile(request.Video, oriVideoPath)
		if err != nil {
			return response, pkgError.InternalServerError(fmt.Sprintf("failed to store video in server %v", err))
		}
	} else {
		return response, pkgError.ValidationError("either VideoUrl or Video must be provided")
	}

	// Check if ffmpeg is installed
	_, err = exec.LookPath("ffmpeg")
	if err != nil {
		return response, pkgError.InternalServerError("ffmpeg not installed")
	}

	// Get thumbnail video with ffmpeg
	thumbnailVideoPath := fmt.Sprintf("%s/%s", config.PathSendItems, generateUUID+".png")
	cmdThumbnail := exec.Command("ffmpeg", "-i", oriVideoPath, "-ss", "00:00:01.000", "-vframes", "1", thumbnailVideoPath)
	err = cmdThumbnail.Run()
	if err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("failed to create thumbnail %v", err))
	}

	// Resize Thumbnail
	srcImage, err := imaging.Open(thumbnailVideoPath)
	if err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("failed to open image %v", err))
	}
	resizedImage := imaging.Resize(srcImage, 100, 0, imaging.Lanczos)
	thumbnailResizeVideoPath := fmt.Sprintf("%s/thumbnails-%s", config.PathSendItems, generateUUID+".png")
	if err = imaging.Save(resizedImage, thumbnailResizeVideoPath); err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("failed to save thumbnail %v", err))
	}

	deletedItems = append(deletedItems, thumbnailVideoPath)
	deletedItems = append(deletedItems, thumbnailResizeVideoPath)
	videoThumbnail = thumbnailResizeVideoPath

	if request.Compress {
		compresVideoPath := fmt.Sprintf("%s/%s", config.PathSendItems, generateUUID+".mp4")

		cmdCompress := exec.Command("ffmpeg", "-i", oriVideoPath, "-strict", "-2", compresVideoPath)
		err = cmdCompress.Run()
		if (err != nil) {
			return response, pkgError.InternalServerError("failed to compress video")
		}

		videoPath = compresVideoPath
		deletedItems = append(deletedItems, compresVideoPath)
	} else {
		videoPath = oriVideoPath
		deletedItems = append(deletedItems, oriVideoPath)
	}

	//Send to WA server
	dataWaVideo, err := os.ReadFile(videoPath)
	if err != nil {
		return response, err
	}
	uploaded, err := service.uploadMedia(ctx, whatsmeow.MediaVideo, dataWaVideo, dataWaRecipient)
	if err != nil {
		return response, pkgError.InternalServerError(fmt.Sprintf("Failed to upload file: %v", err))
	}
	dataWaThumbnail, err := os.ReadFile(videoThumbnail)
	if err != nil {
		return response, err
	}

	msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
		URL:                 proto.String(uploaded.URL),
		Mimetype:            proto.String(http.DetectContentType(dataWaVideo)),
		Caption:             proto.String(request.Caption),
		FileLength:          proto.Uint64(uploaded.FileLength),
		FileSHA256:          uploaded.FileSHA256,
		FileEncSHA256:       uploaded.FileEncSHA256,
		MediaKey:            uploaded.MediaKey,
		DirectPath:          proto.String(uploaded.DirectPath),
		ViewOnce:            proto.Bool(request.ViewOnce),
		JPEGThumbnail:       dataWaThumbnail,
		ThumbnailEncSHA256:  dataWaThumbnail,
		ThumbnailSHA256:     dataWaThumbnail,
		ThumbnailDirectPath: proto.String(uploaded.DirectPath),
	}}
	caption := "🎥 Video"
	if request.Caption != "" {
		caption = "🎥 " + request.Caption
	}
	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, msg, caption)
	go func() {
		errDelete := utils.RemoveFile(1, deletedItems...)
		if errDelete != nil {
			logrus.Infof("error when deleting picture: %v", errDelete)
		}
	}()
	if err != nil {
		return response, err
	}

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Video sent to %s (server timestamp: %s)", request.Phone, ts.Timestamp.String())
	return response, nil
}

func (service serviceSend) SendContact(ctx context.Context, request domainSend.ContactRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendContact(ctx, request)
	if err != nil {
		return response, err
	}
	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	msgVCard := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nN:;%v;;;\nFN:%v\nTEL;type=CELL;waid=%v:+%v\nEND:VCARD",
		request.ContactName, request.ContactName, request.ContactPhone, request.ContactPhone)
	msg := &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
		DisplayName: proto.String(request.ContactName),
		Vcard:       proto.String(msgVCard),
	}}

	content := "👤 " + request.ContactName

	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, msg, content)
	if err != nil {
		return response, err
	}

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Contact sent to %s (server timestamp: %s)", request.Phone, ts.Timestamp.String())
	return response, nil
}

func (service serviceSend) SendLink(ctx context.Context, request domainSend.LinkRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendLink(ctx, request)
	if err != nil {
		return response, err
	}
	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	getMetaDataFromURL := utils.GetMetaDataFromURL(request.Link)

	msg := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text:          proto.String(fmt.Sprintf("%s\n%s", request.Caption, request.Link)),
		Title:         proto.String(getMetaDataFromURL.Title),
		MatchedText:   proto.String(request.Link),
		Description:   proto.String(getMetaDataFromURL.Description),
		JPEGThumbnail: getMetaDataFromURL.ImageThumb,
	}}

	content := "🔗 " + request.Link
	if request.Caption != "" {
		content = "🔗 " + request.Caption
	}
	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, msg, content)
	if err != nil {
		return response, err
	}

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Link sent to %s (server timestamp: %s)", request.Phone, ts.Timestamp.String())
	return response, nil
}

func (service serviceSend) SendLocation(ctx context.Context, request domainSend.LocationRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendLocation(ctx, request)
	if err != nil {
		return response, err
	}
	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	// Compose WhatsApp Proto
	msg := &waE2E.Message{
		LocationMessage: &waE2E.LocationMessage{
			DegreesLatitude:  proto.Float64(utils.StrToFloat64(request.Latitude)),
			DegreesLongitude: proto.Float64(utils.StrToFloat64(request.Longitude)),
		},
	}

	content := "📍 " + request.Latitude + ", " + request.Longitude

	// Send WhatsApp Message Proto
	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, msg, content)
	if err != nil {
		return response, err
	}

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Send location success %s (server timestamp: %s)", request.Phone, ts.Timestamp.String())
	return response, nil
}

func (service serviceSend) SendAudio(ctx context.Context, request domainSend.AudioRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendAudio(ctx, request)
	if err != nil {
		return response, err
	}
	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	autioBytes := helpers.MultipartFormFileHeaderToBytes(request.Audio)
	audioMimeType := http.DetectContentType(autioBytes)

	audioUploaded, err := service.uploadMedia(ctx, whatsmeow.MediaAudio, autioBytes, dataWaRecipient)
	if err != nil {
		err = pkgError.WaUploadMediaError(fmt.Sprintf("Failed to upload audio: %v", err))
		return response, err
	}

	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(audioUploaded.URL),
			DirectPath:    proto.String(audioUploaded.DirectPath),
			Mimetype:      proto.String(audioMimeType),
			FileLength:    proto.Uint64(audioUploaded.FileLength),
			FileSHA256:    audioUploaded.FileSHA256,
			FileEncSHA256: audioUploaded.FileEncSHA256,
			MediaKey:      audioUploaded.MediaKey,
		},
	}

	content := "🎵 Audio"

	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, msg, content)
	if err != nil {
		return response, err
	}

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Send audio success %s (server timestamp: %s)", request.Phone, ts.Timestamp.String())
	return response, nil
}

func (service serviceSend) SendPoll(ctx context.Context, request domainSend.PollRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendPoll(ctx, request)
	if err != nil {
		return response, err
	}
	dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, request.Phone)
	if err != nil {
		return response, err
	}

	content := "📊 " + request.Question

	ts, err := service.wrapSendMessage(ctx, dataWaRecipient, service.WaCli.BuildPollCreation(request.Question, request.Options, request.MaxAnswer), content)
	if err != nil {
		return response, err
	}

	response.MessageID = ts.ID
	response.Status = fmt.Sprintf("Send poll success %s (server timestamp: %s)", request.Phone, ts.Timestamp.String())
	return response, nil
}

func (service serviceSend) SendPresence(ctx context.Context, request domainSend.PresenceRequest) (response domainSend.GenericResponse, err error) {
	err = validations.ValidateSendPresence(ctx, request)
	if err != nil {
		return response, err
	}

	err = service.WaCli.SendPresence(types.Presence(request.Type))
	if err != nil {
		return response, err
	}

	response.MessageID = "presence"
	response.Status = fmt.Sprintf("Send presence success %s", request.Type)
	return response, nil
}

func (service serviceSend) getMentionFromText(_ context.Context, messages string) (result []string) {
	mentions := utils.ContainsMention(messages)
	for _, mention := range mentions {
		// Get JID from phone number
		if dataWaRecipient, err := whatsapp.ValidateJidWithLogin(service.WaCli, mention); err == nil {
			result = append(result, dataWaRecipient.String())
		}
	}
	return result
}

func (service serviceSend) uploadMedia(ctx context.Context, mediaType whatsmeow.MediaType, media []byte, recipient types.JID) (uploaded whatsmeow.UploadResponse, err error) {
	if recipient.Server == types.NewsletterServer {
		uploaded, err = service.WaCli.UploadNewsletter(ctx, media, mediaType)
	} else {
		uploaded, err = service.WaCli.Upload(ctx, media, mediaType)

	}
	return uploaded, err
}
