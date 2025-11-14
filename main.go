// Copyright (c) 2024 Oleg Polozov
// https://github.com/opolozov
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bogem/id3v2"
	"github.com/joho/godotenv"
)

const (
	baseURL               = "https://api.music.yandex.net"
	accountStatusPath     = "/account/status"
	userPlaylistsListPath = "/users/%s/playlists/list"
	userLikesTracksPath   = "/users/%s/likes/tracks"
	trackPath             = "/tracks/%s"
	trackDownloadInfoPath = "/tracks/%s/download-info"
	albumTracksPath       = "/albums/%s/with-tracks"
	userPlaylistPath      = "/users/%s/playlists/%d"
)

// Track представляет трек из плейлиста
type Track struct {
	ID          interface{} `json:"id"`          // Может быть строкой или числом
	RealID      string      `json:"realId"`      // Реальный ID трека
	Title       string      `json:"title"`       // Название трека
	DurationMs  int         `json:"durationMs"`  // Длительность в миллисекундах
	TrackNumber int         `json:"trackNumber"` // Номер трека в альбоме
	Year        int         `json:"year"`        // Год выпуска
	Genre       string      `json:"genre"`       // Жанр
	CoverUri    string      `json:"coverUri"`    // URI обложки альбома
	OgImage     string      `json:"ogImage"`     // Альтернативный URI обложки
	Artists     []struct {
		ID   interface{} `json:"id"`   // Может быть строкой или числом
		Name string      `json:"name"` // Имя исполнителя
	} `json:"artists"`
	Albums []struct {
		ID         interface{} `json:"id"`         // Может быть строкой или числом
		Title      string      `json:"title"`      // Название альбома
		Year       int         `json:"year"`       // Год альбома
		Genre      string      `json:"genre"`      // Жанр альбома
		CoverUri   string      `json:"coverUri"`   // URI обложки альбома
		TrackCount int         `json:"trackCount"` // Количество треков в альбоме
	} `json:"albums"`
}

// TrackShort представляет короткую информацию о треке в плейлисте
type TrackShort struct {
	ID    int   `json:"id"`
	Track Track `json:"track"`
}

// Playlist представляет плейлист
type Playlist struct {
	Owner struct {
		UserID int64  `json:"uid"`
		Login  string `json:"login"`
		Name   string `json:"name"`
	} `json:"owner"`
	Title        string       `json:"title"`
	Kind         int          `json:"kind"`
	PlaylistID   string       `json:"playlistId"`
	PlaylistUuid string       `json:"playlistUuid"`
	Tracks       []TrackShort `json:"tracks"`
	// Дополнительные поля, которые могут быть в ответе
	Available  bool   `json:"available"`
	UserID     int64  `json:"uid"`
	Revision   int    `json:"revision"`
	Snapshot   int    `json:"snapshot"`
	TrackCount int    `json:"trackCount"`
	Visibility string `json:"visibility"`
	Collective bool   `json:"collective"`
	Created    string `json:"created"`
}

// PlaylistResponse представляет ответ API для плейлиста
type PlaylistResponse struct {
	Result []Playlist `json:"result"`
}

// AccountInfo представляет информацию об аккаунте
type AccountInfo struct {
	UserID      int64  `json:"uid"`
	Login       string `json:"login"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// GetUserID возвращает UserID как строку
func (a AccountInfo) GetUserID() string {
	return fmt.Sprintf("%d", a.UserID)
}

// AccountStatus представляет информацию об аккаунте
type AccountStatus struct {
	Result struct {
		Account AccountInfo `json:"account"`
	} `json:"result"`
}

// YandexMusicClient представляет клиент для работы с API Яндекс.Музыки
type YandexMusicClient struct {
	token  string
	client *http.Client
}

// NewClient создает новый клиент Яндекс.Музыки
func NewClient(token string) *YandexMusicClient {
	return &YandexMusicClient{
		token:  token,
		client: &http.Client{},
	}
}

// makeRequest выполняет HTTP запрос к API
func (c *YandexMusicClient) makeRequest(method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ошибка API: статус %d, ответ: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

// setHeaders устанавливает стандартные заголовки для запросов
func (c *YandexMusicClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "OAuth "+c.token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
}

// GetAccountStatus получает информацию о текущем пользователе
func (c *YandexMusicClient) GetAccountStatus() (*AccountStatus, error) {
	url := baseURL + accountStatusPath
	resp, err := c.makeRequest("GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var status AccountStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	return &status, nil
}

// GetUserPlaylists получает список плейлистов пользователя
func (c *YandexMusicClient) GetUserPlaylists(userID string) ([]Playlist, error) {
	// Если userID пустой или "me", получаем userId из account/status
	if userID == "" || userID == "me" {
		account, err := c.GetAccountStatus()
		if err != nil {
			return nil, fmt.Errorf("не удалось получить userId пользователя: %w", err)
		}
		userID = account.Result.Account.GetUserID()
		if userID == "" {
			return nil, fmt.Errorf("userId пользователя пустой")
		}
	}
	url := baseURL + fmt.Sprintf(userPlaylistsListPath, userID)
	resp, err := c.makeRequest("GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var response struct {
		Result []Playlist `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	return response.Result, nil
}

// GetLikedTracks получает список избранных треков (лайков) пользователя
func (c *YandexMusicClient) GetLikedTracks(userID string) ([]TrackShort, error) {
	// Если userID пустой или "me", получаем userId из account/status
	if userID == "" || userID == "me" {
		account, err := c.GetAccountStatus()
		if err != nil {
			return nil, fmt.Errorf("не удалось получить userId пользователя: %w", err)
		}
		userID = account.Result.Account.GetUserID()
		if userID == "" {
			return nil, fmt.Errorf("userId пользователя пустой")
		}
	}

	url := baseURL + fmt.Sprintf(userLikesTracksPath, userID)
	resp, err := c.makeRequest("GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var response struct {
		Result struct {
			Library struct {
				Tracks []struct {
					ID      string `json:"id"`
					AlbumID string `json:"albumId"`
				} `json:"tracks"`
			} `json:"library"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	tracks := make([]TrackShort, 0, len(response.Result.Library.Tracks))
	for _, trackRef := range response.Result.Library.Tracks {
		// Получаем полную информацию о треке
		track, err := c.getTrackByID(trackRef.ID)
		if err != nil {
			log.Printf("Ошибка получения трека %s: %v\n", trackRef.ID, err)
			continue
		}
		tracks = append(tracks, TrackShort{
			ID:    0, // Будет заполнено из track
			Track: *track,
		})
	}

	return tracks, nil
}

// getTrackByID получает полную информацию о треке по ID
func (c *YandexMusicClient) getTrackByID(trackID string) (*Track, error) {
	url := baseURL + fmt.Sprintf(trackPath, trackID)
	resp, err := c.makeRequest("GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var response struct {
		Result []Track `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	if len(response.Result) == 0 {
		return nil, fmt.Errorf("трек не найден")
	}

	return &response.Result[0], nil
}

// GetAlbumTracks получает список треков альбома
func (c *YandexMusicClient) GetAlbumTracks(playlistID string) ([]Track, error) {
	url := baseURL + fmt.Sprintf(albumTracksPath, playlistID)
	resp, err := c.makeRequest("GET", url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var response struct {
		Result struct {
			Volumes [][]Track `json:"volumes"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	var tracks []Track
	for _, volume := range response.Result.Volumes {
		tracks = append(tracks, volume...)
	}

	return tracks, nil
}

// GetPlaylistTracks получает список треков плейлиста по ID
func (c *YandexMusicClient) GetPlaylistTracks(playlistID string) ([]TrackShort, error) {
	// Получаем userId
	account, err := c.GetAccountStatus()
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении userId: %w", err)
	}
	userID := account.Result.Account.GetUserID()
	if userID == "" {
		return nil, fmt.Errorf("userId пользователя пустой")
	}

	// Парсим playlistID - может быть kind (число) или UUID
	var kind int
	if k, err := strconv.Atoi(playlistID); err == nil {
		kind = k
	} else {
		// Если не число, ищем плейлист по UUID
		playlists, err := c.GetUserPlaylists(userID)
		if err != nil {
			return nil, fmt.Errorf("ошибка при получении списка плейлистов: %w", err)
		}
		found := false
		for _, p := range playlists {
			if p.PlaylistUuid == playlistID || p.PlaylistID == playlistID {
				kind = p.Kind
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("плейлист с ID %s не найден", playlistID)
		}
	}

	// Получаем плейлист по kind
	url := baseURL + fmt.Sprintf(userPlaylistPath, userID, kind)
	resp, err := c.makeRequest("GET", url)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении плейлиста: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var response struct {
		Result Playlist `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	return response.Result.Tracks, nil
}

// GetTrackDownloadURL получает ссылку на MP3 для скачивания трека
func (c *YandexMusicClient) GetTrackDownloadURL(trackID string) (string, error) {
	url := baseURL + fmt.Sprintf(trackDownloadInfoPath, trackID)
	resp, err := c.makeRequest("GET", url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var response struct {
		Result []struct {
			Codec           string `json:"codec"`
			Bitrate         int    `json:"bitrate"`
			Gain            bool   `json:"gain"`
			Preview         bool   `json:"preview"`
			DownloadInfoURL string `json:"downloadInfoUrl"`
			Direct          bool   `json:"direct"`
			Barcode         string `json:"barcode"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	if len(response.Result) == 0 {
		return "", fmt.Errorf("нет доступных ссылок для скачивания")
	}

	// Берем первую доступную ссылку (обычно лучшего качества)
	downloadInfoURL := response.Result[0].DownloadInfoURL
	if downloadInfoURL == "" {
		return "", fmt.Errorf("ссылка на скачивание не найдена")
	}

	// Получаем прямую ссылку на MP3 с авторизацией
	downloadReq, err := http.NewRequest("GET", downloadInfoURL, nil)
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}
	c.setHeaders(downloadReq)

	downloadResp, err := c.client.Do(downloadReq)
	if err != nil {
		return "", fmt.Errorf("ошибка получения ссылки на скачивание: %w", err)
	}
	defer downloadResp.Body.Close()

	downloadBody, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	var downloadInfo struct {
		XMLName xml.Name `xml:"download-info"`
		Host    string   `xml:"host"`
		Path    string   `xml:"path"`
		S       string   `xml:"s"`
		Ts      string   `xml:"ts"`
	}
	if err := xml.Unmarshal(downloadBody, &downloadInfo); err != nil {
		return "", fmt.Errorf("ошибка декодирования информации о скачивании: %w", err)
	}

	// Формируем прямую ссылку на MP3
	mp3URL := fmt.Sprintf("https://%s/get-mp3/%s/%s/%s", downloadInfo.Host, downloadInfo.S, downloadInfo.Ts, downloadInfo.Path)
	return mp3URL, nil
}

func main() {
	// Парсим аргументы командной строки
	var (
		command    = flag.String("cmd", "", "Команда: playlist, likes, list-playlists, download-playlist")
		playlistID = flag.String("id", "", "ID плейлиста для команды playlist или download-playlist")
		outputFmt  = flag.String("out", "", "Формат вывода: json (по умолчанию - текст)")
		folderName = flag.String("to", "", "Папка для сохранения (для команды download-playlist)")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Использование: %s [опции]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Команды:\n")
		fmt.Fprintf(os.Stderr, "  -cmd=playlist -id=ID [-out=json] Просмотреть список всех песен плейлиста с ссылками на MP3\n")
		fmt.Fprintf(os.Stderr, "  -cmd=likes [-out=json]           Просмотреть список избранного с ссылками на MP3\n")
		fmt.Fprintf(os.Stderr, "  -cmd=list-playlists [-out=json]   Просмотреть список всех плейлистов\n")
		fmt.Fprintf(os.Stderr, "  -cmd=download-playlist -id=ID -to=folder Скачать все песни плейлиста в папку\n\n")
		fmt.Fprintf(os.Stderr, "Примеры:\n")
		fmt.Fprintf(os.Stderr, "  yandex-music-exporter -cmd=playlist -id=12345\n")
		fmt.Fprintf(os.Stderr, "  yandex-music-exporter -cmd=playlist -id=12345 -out=json\n")
		fmt.Fprintf(os.Stderr, "  yandex-music-exporter -cmd=likes\n")
		fmt.Fprintf(os.Stderr, "  yandex-music-exporter -cmd=list-playlists\n")
		fmt.Fprintf(os.Stderr, "  yandex-music-exporter -cmd=list-playlists -out=json\n")
		fmt.Fprintf(os.Stderr, "  yandex-music-exporter -cmd=download-playlist -id=12345 -to=./music\n\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// Загрузка переменных окружения из .env файла
	if err := godotenv.Load(); err != nil {
		log.Printf("Предупреждение: не удалось загрузить .env файл: %v", err)
	}

	// Получаем токен доступа
	token := os.Getenv("ACCESS_TOKEN")
	if token == "" {
		log.Fatal("Ошибка: ACCESS_TOKEN не найден в .env файле или переменных окружения")
	}

	// Создаем клиент
	client := NewClient(token)

	// Обрабатываем команды
	if *command == "" {
		flag.Usage()
		log.Fatal("Ошибка: необходимо указать команду через флаг -cmd")
	}

	switch *command {
	case "playlist":
		if *playlistID == "" {
			log.Fatal("Ошибка: для команды 'playlist' необходимо указать ID плейлиста через флаг -id")
		}
		handlePlaylistTracks(client, *playlistID, *outputFmt)
	case "likes", "favorites":
		handleLikes(client, *outputFmt)
	case "list-playlists":
		handleListPlaylists(client, *outputFmt)
	case "download-playlist":
		if *playlistID == "" {
			log.Fatal("Ошибка: для команды 'download-playlist' необходимо указать ID плейлиста через флаг -id")
		}
		if *folderName == "" {
			log.Fatal("Ошибка: для команды 'download-playlist' необходимо указать папку через флаг -to")
		}
		handleDownloadPlaylist(client, *playlistID, *folderName)
	case "download-likes":
		if *folderName == "" {
			log.Fatal("Ошибка: для команды 'download-likes' необходимо указать папку через флаг -to")
		}
		handleDownloadLikes(client, *folderName)
	default:
		log.Fatalf("Неизвестная команда: %s. Доступные команды: playlist, likes, list-playlists, download-playlist, download-likes", *command)
	}
}

// handlePlaylistTracks обрабатывает команду playlist
func handlePlaylistTracks(client *YandexMusicClient, playlistID string, outputFmt string) {
	tracks, err := client.GetPlaylistTracks(playlistID)
	if err != nil {
		log.Fatalf("Ошибка при получении треков плейлиста: %v\n", err)
	}

	// Подготавливаем данные для вывода
	type TrackOutput struct {
		Title  string `json:"title"`
		Artist string `json:"artist"`
		Link   string `json:"link"`
	}

	var tracksOutput []TrackOutput
	for _, trackShort := range tracks {
		track := trackShort.Track
		artistNames := []string{}
		for _, artist := range track.Artists {
			artistNames = append(artistNames, artist.Name)
		}
		artistStr := strings.Join(artistNames, ", ")
		if artistStr == "" {
			artistStr = "Неизвестный исполнитель"
		}

		trackIDStr := fmt.Sprintf("%v", track.ID)

		// Получаем ссылку на MP3
		mp3URL, err := client.GetTrackDownloadURL(trackIDStr)
		if err != nil {
			log.Printf("Ошибка получения ссылки для трека %s: %v\n", track.Title, err)
			mp3URL = ""
		}

		trackName := fmt.Sprintf("%s — %s", track.Title, artistStr)
		tracksOutput = append(tracksOutput, TrackOutput{
			Title:  track.Title,
			Artist: artistStr,
			Link:   mp3URL,
		})

		// Вывод в зависимости от формата
		if outputFmt == "json" {
			// JSON вывод будет после цикла
		} else {
			// Текстовый формат: {trackname} \t {link}
			fmt.Printf("%s\t%s\n", trackName, mp3URL)
		}
	}

	// JSON вывод
	if outputFmt == "json" {
		jsonData, err := json.MarshalIndent(tracksOutput, "", "  ")
		if err != nil {
			log.Fatalf("Ошибка формирования JSON: %v\n", err)
		}
		fmt.Println(string(jsonData))
	}
}

// handleLikes обрабатывает команду likes
func handleLikes(client *YandexMusicClient, outputFmt string) {
	likedTracks, err := client.GetLikedTracks("")
	if err != nil {
		log.Fatalf("Ошибка при получении избранных треков: %v\n", err)
	}

	// Подготавливаем данные для вывода
	type TrackOutput struct {
		Title  string `json:"title"`
		Artist string `json:"artist"`
		Link   string `json:"link"`
	}

	var tracksOutput []TrackOutput
	for _, trackShort := range likedTracks {
		artistNames := []string{}
		for _, artist := range trackShort.Track.Artists {
			artistNames = append(artistNames, artist.Name)
		}
		artistStr := strings.Join(artistNames, ", ")
		if artistStr == "" {
			artistStr = "Неизвестный исполнитель"
		}

		trackIDStr := fmt.Sprintf("%v", trackShort.Track.ID)

		// Получаем ссылку на MP3
		mp3URL, err := client.GetTrackDownloadURL(trackIDStr)
		if err != nil {
			log.Printf("Ошибка получения ссылки для трека %s: %v\n", trackShort.Track.Title, err)
			mp3URL = ""
		}

		trackName := fmt.Sprintf("%s — %s", trackShort.Track.Title, artistStr)
		tracksOutput = append(tracksOutput, TrackOutput{
			Title:  trackShort.Track.Title,
			Artist: artistStr,
			Link:   mp3URL,
		})

		// Вывод в зависимости от формата
		if outputFmt == "json" {
			// JSON вывод будет после цикла
		} else {
			// Текстовый формат: {trackname} \t {link}
			fmt.Printf("%s\t%s\n", trackName, mp3URL)
		}
	}

	// JSON вывод
	if outputFmt == "json" {
		jsonData, err := json.MarshalIndent(tracksOutput, "", "  ")
		if err != nil {
			log.Fatalf("Ошибка формирования JSON: %v\n", err)
		}
		fmt.Println(string(jsonData))
	}
}

// handleListPlaylists обрабатывает команду list-playlists
func handleListPlaylists(client *YandexMusicClient, outputFmt string) {
	playlists, err := client.GetUserPlaylists("")
	if err != nil {
		log.Fatalf("Ошибка при получении списка плейлистов: %v\n", err)
	}

	// Подготавливаем данные для вывода
	type PlaylistOutput struct {
		Title  string `json:"title"`
		ID     string `json:"id"`
		UUID   string `json:"uuid,omitempty"`
		Kind   int    `json:"kind,omitempty"`
		Tracks int    `json:"tracks,omitempty"`
	}

	var playlistsOutput []PlaylistOutput
	for _, playlist := range playlists {
		// Определяем ID (приоритет UUID, затем Kind)
		playlistID := ""
		if playlist.PlaylistUuid != "" {
			playlistID = playlist.PlaylistUuid
		} else if playlist.Kind != 0 {
			playlistID = fmt.Sprintf("%d", playlist.Kind)
		}

		playlistsOutput = append(playlistsOutput, PlaylistOutput{
			Title:  playlist.Title,
			ID:     playlistID,
			UUID:   playlist.PlaylistUuid,
			Kind:   playlist.Kind,
			Tracks: playlist.TrackCount,
		})

		// Вывод в зависимости от формата
		if outputFmt == "json" {
			// JSON вывод будет после цикла
		} else {
			// Текстовый формат: {title} \t {id}
			fmt.Printf("%s\t%s\n", playlist.Title, playlistID)
		}
	}

	// JSON вывод
	if outputFmt == "json" {
		jsonData, err := json.MarshalIndent(playlistsOutput, "", "  ")
		if err != nil {
			log.Fatalf("Ошибка формирования JSON: %v\n", err)
		}
		fmt.Println(string(jsonData))
	}
}

// handleDownloadPlaylist обрабатывает команду download-playlist
func handleDownloadPlaylist(client *YandexMusicClient, playlistID string, folderName string) {
	tracks, err := client.GetPlaylistTracks(playlistID)
	if err != nil {
		log.Fatalf("Ошибка при получении треков плейлиста: %v\n", err)
	}

	fmt.Printf("Найдено треков в плейлисте: %d\n", len(tracks))
	downloadTracks(client, tracks, folderName)
}

// handleDownloadLikes обрабатывает команду download-likes
func handleDownloadLikes(client *YandexMusicClient, folderName string) {
	tracks, err := client.GetLikedTracks("")
	if err != nil {
		log.Fatalf("Ошибка при получении лайкнутых треков: %v\n", err)
	}

	fmt.Printf("Найдено лайкнутых треков: %d\n", len(tracks))
	downloadTracks(client, tracks, folderName)
}

// downloadTracks скачивает список треков в указанную папку
func downloadTracks(client *YandexMusicClient, tracks []TrackShort, folderName string) {
	// Создаем папку, если её нет
	if err := os.MkdirAll(folderName, 0755); err != nil {
		log.Fatalf("Ошибка создания папки %s: %v\n", folderName, err)
	}

	fmt.Printf("Папка для сохранения: %s\n\n", folderName)

	downloaded := 0
	skipped := 0
	failed := 0

	for i, trackShort := range tracks {
		track := trackShort.Track
		artistNames := []string{}
		for _, artist := range track.Artists {
			artistNames = append(artistNames, artist.Name)
		}
		artistStr := strings.Join(artistNames, ", ")
		if artistStr == "" {
			artistStr = "Неизвестный исполнитель"
		}

		// Формируем имя файла: {исполнитель}-{песня}.mp3
		// Очищаем от недопустимых символов для имени файла
		fileName := sanitizeFileName(fmt.Sprintf("%s-%s.mp3", artistStr, track.Title))
		filePath := filepath.Join(folderName, fileName)

		// Проверяем, существует ли файл
		if _, err := os.Stat(filePath); err == nil {
			fmt.Printf("[%d/%d] Пропущено (уже существует): %s — %s\n", i+1, len(tracks), track.Title, artistStr)
			skipped++
			continue
		}

		// Получаем ссылку на MP3
		trackIDStr := fmt.Sprintf("%v", track.ID)
		mp3URL, err := client.GetTrackDownloadURL(trackIDStr)
		if err != nil {
			fmt.Printf("[%d/%d] Ошибка получения ссылки: %s — %s (%v)\n", i+1, len(tracks), track.Title, artistStr, err)
			failed++
			continue
		}

		// Скачиваем файл
		lastProgress := -1.0
		progressPrefix := fmt.Sprintf("[%d/%d] Скачивание: %s — %s", i+1, len(tracks), track.Title, artistStr)
		if err := downloadFileWithProgress(mp3URL, filePath, client.token, func(progress float64) {
			// Обновляем прогресс только если изменился на 0.5% или больше
			if progress-lastProgress >= 0.5 || progress >= 100.0 {
				// Используем ANSI escape-код для очистки до конца строки и \r для возврата каретки
				fmt.Fprintf(os.Stdout, "\r\033[K%s %.1f%%", progressPrefix, progress)
				os.Stdout.Sync() // Принудительно выводим буфер
				lastProgress = progress
			}
		}); err != nil {
			// Очищаем строку перед выводом ошибки
			fmt.Fprintf(os.Stdout, "\r\033[K")
			fmt.Printf("[%d/%d] ✗ Ошибка скачивания: %s — %s (%v)\n", i+1, len(tracks), track.Title, artistStr, err)
			failed++
			continue
		}

		// Записываем ID3 теги
		if err := writeID3Tags(filePath, track); err != nil {
			fmt.Printf("[%d/%d] Предупреждение: не удалось записать ID3 теги для %s — %s (%v)\n", i+1, len(tracks), track.Title, artistStr, err)
		}

		// Очищаем строку и выводим результат
		fmt.Fprintf(os.Stdout, "\r\033[K")
		fmt.Printf("[%d/%d] ✓ Сохранено: %s\n", i+1, len(tracks), fileName)
		downloaded++
	}

	fmt.Printf("\nГотово!\n")
	fmt.Printf("Скачано: %d\n", downloaded)
	fmt.Printf("Пропущено: %d\n", skipped)
	fmt.Printf("Ошибок: %d\n", failed)
}

// sanitizeFileName очищает имя файла от недопустимых символов
func sanitizeFileName(name string) string {
	// Заменяем недопустимые символы на подчеркивание
	invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range invalidChars {
		result = strings.ReplaceAll(result, char, "_")
	}
	// Удаляем множественные подчеркивания
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}
	return result
}

// downloadFile скачивает файл по URL и сохраняет его
func downloadFile(url string, filePath string, token string) error {
	return downloadFileWithProgress(url, filePath, token, nil)
}

// downloadFileWithProgress скачивает файл по URL с отображением прогресса
func downloadFileWithProgress(url string, filePath string, token string, progressCallback func(float64)) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "OAuth "+token)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ошибка HTTP: статус %d", resp.StatusCode)
	}

	// Создаем файл
	outFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("ошибка создания файла: %w", err)
	}
	defer outFile.Close()

	// Получаем размер файла
	totalSize := resp.ContentLength
	var downloaded int64

	// Копируем данные с отслеживанием прогресса
	buf := make([]byte, 32*1024) // 32KB буфер
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := outFile.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			downloaded += int64(nw)
			if ew != nil {
				return fmt.Errorf("ошибка записи файла: %w", ew)
			}
			if nr != nw {
				return fmt.Errorf("ошибка записи: неполная запись")
			}

			// Вызываем callback для обновления прогресса
			if progressCallback != nil && totalSize > 0 {
				progress := float64(downloaded) / float64(totalSize) * 100
				progressCallback(progress)
			}
		}
		if er != nil {
			if er != io.EOF {
				return fmt.Errorf("ошибка чтения: %w", er)
			}
			break
		}
	}

	// Финальный прогресс 100%
	if progressCallback != nil && totalSize > 0 {
		progressCallback(100.0)
	}

	return nil
}

// writeID3Tags записывает ID3 теги в MP3 файл
func writeID3Tags(filePath string, track Track) error {
	// Открываем файл для записи тегов
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("ошибка открытия файла для записи тегов: %v", err)
	}
	defer tag.Close()

	// Записываем название трека
	if track.Title != "" {
		tag.SetTitle(track.Title)
	}

	// Записываем исполнителей
	artistNames := []string{}
	for _, artist := range track.Artists {
		if artist.Name != "" {
			artistNames = append(artistNames, artist.Name)
		}
	}
	if len(artistNames) > 0 {
		tag.SetArtist(strings.Join(artistNames, ", "))
	}

	// Записываем альбом (берем первый альбом, если есть)
	if len(track.Albums) > 0 && track.Albums[0].Title != "" {
		tag.SetAlbum(track.Albums[0].Title)
	}

	// Записываем год (приоритет: год трека, затем год альбома)
	year := track.Year
	if year == 0 && len(track.Albums) > 0 {
		year = track.Albums[0].Year
	}
	if year > 0 {
		tag.SetYear(strconv.Itoa(year))
	}

	// Записываем номер трека в альбоме
	if track.TrackNumber > 0 {
		trackNumberStr := strconv.Itoa(track.TrackNumber)
		// Если есть информация о количестве треков в альбоме, добавляем её
		if len(track.Albums) > 0 && track.Albums[0].TrackCount > 0 {
			trackNumberStr = fmt.Sprintf("%d/%d", track.TrackNumber, track.Albums[0].TrackCount)
		}
		trackFrame := id3v2.TextFrame{
			Encoding: tag.DefaultEncoding(),
			Text:     trackNumberStr,
		}
		tag.AddFrame("TRCK", trackFrame)
	}

	// Записываем жанр (приоритет: жанр трека, затем жанр альбома)
	genre := track.Genre
	if genre == "" && len(track.Albums) > 0 {
		genre = track.Albums[0].Genre
	}
	if genre != "" {
		tag.SetGenre(genre)
	}

	// Записываем URI обложки альбома в пользовательский URL фрейм (WXXX)
	coverURI := track.CoverUri
	if coverURI == "" {
		coverURI = track.OgImage
	}
	if coverURI == "" && len(track.Albums) > 0 {
		coverURI = track.Albums[0].CoverUri
	}
	if coverURI != "" {
		// Формируем полный URL обложки (если это относительный путь)
		coverURL := coverURI
		if !strings.HasPrefix(coverURI, "http://") && !strings.HasPrefix(coverURI, "https://") {
			coverURL = "https://" + strings.TrimPrefix(coverURI, "//")
		}
		// Записываем URI в пользовательский URL фрейм
		urlFrame := id3v2.URLUserDefinedFrame{
			Encoding:    tag.DefaultEncoding(),
			Description: "Cover Art URL",
			URL:         coverURL,
		}
		tag.AddFrame("WXXX", urlFrame)
	}

	// Сохраняем изменения
	if err := tag.Save(); err != nil {
		return fmt.Errorf("ошибка сохранения тегов: %v", err)
	}

	return nil
}
