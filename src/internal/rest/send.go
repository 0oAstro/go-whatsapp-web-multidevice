package rest

import (
	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/whatsapp"
	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
)

type Send struct {
	Service domainSend.ISendService
}

func InitRestSend(app *fiber.App, service domainSend.ISendService) Send {
	rest := Send{Service: service}
	app.Post("/send/message", rest.SendText)
	app.Post("/send/image", rest.SendImage)
	app.Post("/send/file", rest.SendFile)
	app.Post("/send/video", rest.SendVideo)
	app.Post("/send/contact", rest.SendContact)
	app.Post("/send/link", rest.SendLink)
	app.Post("/send/location", rest.SendLocation)
	app.Post("/send/audio", rest.SendAudio)
	app.Post("/send/poll", rest.SendPoll)
	return rest
}

func (controller *Send) SendText(c *fiber.Ctx) error {
	var request domainSend.MessageRequest
	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendText(c.UserContext(), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}

func (controller *Send) SendImage(c *fiber.Ctx) error {
	var request domainSend.ImageRequest
	request.Compress = true

	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	// Add debug logging
	logrus.WithFields(logrus.Fields{
		"body": c.Body(),
		"form": c.FormValue("image_url"),
	}).Debug("Image request received")

	request.ImageUrl = c.FormValue("image_url")
	if request.ImageUrl == "" {
		if file, err := c.FormFile("image"); err == nil {
			request.Image = file
			logrus.WithField("filename", file.Filename).Debug("Image file received")
		} else {
			logrus.WithError(err).Debug("No image file found")
		}
	} else {
		logrus.WithField("url", request.ImageUrl).Debug("Image URL received")
	}

	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendImage(c.UserContext(), request)
	if err != nil {
		logrus.WithError(err).Error("Failed to send image")
		return err
	}

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}

func (controller *Send) SendFile(c *fiber.Ctx) error {
	var request domainSend.FileRequest
	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	request.FileUrl = c.FormValue("file_url")
	if request.FileUrl == "" {
		if file, err := c.FormFile("file"); err == nil {
			request.File = file
		}
	}
	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendFile(c.UserContext(), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}

func (controller *Send) SendVideo(c *fiber.Ctx) error {
	var request domainSend.VideoRequest
	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	request.VideoUrl = c.FormValue("video_url")
	if request.VideoUrl == "" {
		if video, err := c.FormFile("video"); err == nil {
			request.Video = video
		}
	}
	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendVideo(c.UserContext(), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}

func (controller *Send) SendContact(c *fiber.Ctx) error {
	var request domainSend.ContactRequest
	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendContact(c.UserContext(), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}

func (controller *Send) SendLink(c *fiber.Ctx) error {
	var request domainSend.LinkRequest
	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendLink(c.UserContext(), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}

func (controller *Send) SendLocation(c *fiber.Ctx) error {
	var request domainSend.LocationRequest
	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendLocation(c.UserContext(), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}

func (controller *Send) SendAudio(c *fiber.Ctx) error {
	var request domainSend.AudioRequest
	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	audio, err := c.FormFile("audio")
	utils.PanicIfNeeded(err)

	request.Audio = audio
	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendAudio(c.UserContext(), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}

func (controller *Send) SendPoll(c *fiber.Ctx) error {
	var request domainSend.PollRequest
	err := c.BodyParser(&request)
	utils.PanicIfNeeded(err)

	whatsapp.SanitizePhone(&request.Phone)

	response, err := controller.Service.SendPoll(c.UserContext(), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Status,
		Results: response,
	})
}
