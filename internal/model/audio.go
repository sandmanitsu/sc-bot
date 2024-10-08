package model

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os/exec"
	"sc-bot/internal/disk"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

var (
	isPlaying bool
	mutex     sync.Mutex
	voice     *discordgo.VoiceConnection
	ffmpeg    *exec.Cmd
)

// google drive folder id
const (
	RainyNightsOf1988 = "1KaLJMxkFQ8daK39Sl8Do6jgeFDTkDlD7"
	OtherTracks       = "1ORRwndJayZhgSB0ZQ9aT7W0D3intMOVf"
)

func Play(s *discordgo.Session, id string, channelId string) string {
	mutex.Lock()
	if isPlaying {
		mutex.Unlock()
		return "Track already play, use /stop"
	}
	isPlaying = true
	mutex.Unlock()

	audioURL := "https://drive.google.com/uc?export=download&id=" + id

	voice, err := s.ChannelVoiceJoin(GuildId, channelId, false, false)
	if err != nil {
		log.Fatalf("Error joining voice channel: %v", err)
	}
	defer voice.Disconnect()

	ffmpeg = exec.Command("ffmpeg", "-i", audioURL, "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpegStdout, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Fatalf("Error creating StdoutPipe for FFmpeg: %v", err)
	}

	err = ffmpeg.Start()
	if err != nil {
		log.Fatalf("Error starting FFmpeg: %v", err)
	}

	voice.Speaking(true)
	defer voice.Speaking(false)

	opusEncoder, err := gopus.NewEncoder(48000, 2, gopus.Audio)
	if err != nil {
		log.Fatalf("Error creating Opus encoder: %v", err)
	}

	buffer := make([]byte, 960*2*2) // Buffer 20ms 48kHz stereo PCM
	for {
		n, err := ffmpegStdout.Read(buffer)
		if n > 0 {
			pcmData := make([]int16, n/2)
			for i := 0; i < len(pcmData); i++ {
				pcmData[i] = int16(binary.LittleEndian.Uint16(buffer[i*2 : (i+1)*2]))
			}

			opusData, err := opusEncoder.Encode(pcmData, 960, 8000)
			if err != nil {
				log.Fatalf("Error encoding PCM to Opus: %v", err)
			}
			voice.OpusSend <- opusData
		}
		if err == io.EOF {
			mutex.Lock()
			isPlaying = false
			mutex.Unlock()

			fmt.Println("End of stream")
			break
		}
		if err != nil {
			log.Fatalf("Error reading from FFmpeg stdout: %v", err)
		}
	}

	err = ffmpeg.Wait()
	if err != nil {
		fmt.Printf("Error ending ffmpeg: %v", err)
	}
	return "Finished playing audio"
}

func GetChoices() []*discordgo.ApplicationCommandOptionChoice {
	service, err := disk.GetService()
	if err != nil {
		fmt.Printf("Error getting service: %v", err)
	}

	// todo может быть стоит перенести набор папок в конфиг????
	list := disk.ListFilesInFolder(service, RainyNightsOf1988) // folder: Rainy Nights Of 1988
	choices := []*discordgo.ApplicationCommandOptionChoice{}

	for _, v := range list {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  formatName(v.Name),
			Value: v.Id,
		})
	}

	return choices
}

func formatName(name string) string {
	formName := strings.Split(name, "-")

	if len(formName) == 2 {
		return strings.TrimSpace(formName[1])
	}

	return "can't read a name"
}

func Stop() string {
	mutex.Lock()
	defer mutex.Unlock()

	if isPlaying {
		// Прерываем процесс ffmpeg
		if ffmpeg != nil && ffmpeg.Process != nil {
			ffmpeg.Process.Kill()
		}
		isPlaying = false
	}

	// Отключаем бота от голосового канала
	if voice != nil {
		voice.Disconnect()
		voice = nil
	}

	return "Stop playing audio"
}

func GetRandomTrack() string {
	service, err := disk.GetService()
	if err != nil {
		fmt.Printf("Error getting service: %v", err)
	}

	list := disk.ListFilesInFolder(service, OtherTracks)

	return list[randomNumber(len(list))].Id
}

func randomNumber(max int) int {
	// random number generator
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	min := 0

	return r.Intn(max-min+1) + min
}

func Autoplay(s *discordgo.Session, channelId string) string {
	service, err := disk.GetService()
	if err != nil {
		fmt.Printf("Error getting service: %v", err)
	}

	list := disk.ListFilesInFolder(service, OtherTracks)
	countTraks := len(list)

	for i := 0; i < countTraks; i++ {
		_ = Play(s, list[randomNumber(countTraks)].Id, channelId)
	}

	return "Finished playing list of tracks"
}
