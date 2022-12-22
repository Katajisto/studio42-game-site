package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/gomarkdown/markdown"
)

// Compile templates on start of the application
var uploadTemplate = template.Must(template.ParseFiles("upload.html"))

// Display the named template
func uploadPage(w http.ResponseWriter, r *http.Request) {
	// get auth from url and pass it in a struct to the tempalate
	auth := r.URL.Query().Get("auth")
	
	game := r.URL.Query().Get("id")
	if game == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	uploadTemplate.Execute(w, struct {
		Auth string
		Game string
	}{auth, game})
}

// Loads markdown data from pocketbase.

type cachedMd struct {
	Md      string
	Fetched time.Time
}

var markdownCache map[string]cachedMd

var playTemplate *template.Template

func init() {
	markdownCache = make(map[string]cachedMd)
}

var myClient = &http.Client{Timeout: 10 * time.Second}

func getJson(url string, target interface{}) error {
	r, err := myClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

type Game struct {
	Title        string `json:"name"`
	Desc         string `json:"description"`
	Folder       string `json:"file_folder"`
	CollectionId string `json:"@collectionId"`
	Id           string `json:"id"`
	Img          string `json:"img"`
	Builds       []int
}

func getGameList() []Game {
	type reqData struct {
		Games []Game `json:"items"`
	}

	var data reqData
	getJson("https://based.0x2a.fi/api/collections/studio42_game_list/records", &data)

	return data.Games
}

func getMainPageMd() string {
	type PbMainPageData struct {
		MainText string `json:"main_text"`
	}

	var data PbMainPageData
	getJson("https://based.0x2a.fi/api/collections/studio42_pagedata/records/hv124z72j9e48zb", &data)
	return data.MainText
}

func playPage(w http.ResponseWriter, r *http.Request) {
	game := r.URL.Query().Get("id")
	build := r.URL.Query().Get("build")

	if game == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	type PlayData struct {
		Builds   []string
		CurBuild string
		Game     string
	}

	builds := getBuildArr(game)

	buildToPlay := builds[0]
	if build != "" {
		buildToPlay = build
	}

	playTemplate.Execute(w, PlayData{builds, buildToPlay, game})
}

func getBuildArr(id string) []string {
	files, err := ioutil.ReadDir("./games/" + id + "/")
	if err != nil {
		log.Println("Error in looking for game builds: ", err)
	}

	builds := []string{}

	// Go trough builds for the game
	for _, buildDir := range files {
		// Filter out non-folders that might for some reason be there.
		if !buildDir.IsDir() {
			continue
		}

		builds = append(builds, buildDir.Name())
	}

	sort.Sort(sort.Reverse(sort.StringSlice(builds)))

	return builds
}

func uploadFile(w http.ResponseWriter, r *http.Request) {
	auth := r.URL.Query().Get("auth")
	if auth != os.Getenv("AUTH") {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, "Unauthorized")
		return
	}

	game := r.URL.Query().Get("id")
	if game == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	r.ParseMultipartForm(500 << 20)

	file, _, err := r.FormFile("game")
	if err != nil {
		fmt.Println("Error Retrieving the File")
		fmt.Println(err)
		return
	}

	defer file.Close()

	buf := &bytes.Buffer{}
	nRead, err := io.Copy(buf, file)
	if err != nil {
		fmt.Println(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), nRead)

	buildArr := getBuildArr(game)

	nextVersion := 1
	if len(buildArr) > 0 {
		latest, _ := strconv.Atoi(buildArr[0])
		nextVersion = latest + 1
		fmt.Println(latest, nextVersion)
	}

	Unzip(zr, fmt.Sprintf("./games/%v/%v/", game, nextVersion))

	fmt.Fprintf(w, "Successfully Uploaded File\n")
}

func main() {
	fmt.Println("STARTING STUDIO42.FI SERVER ON PORT 1337")
	homeTemplate := template.Must(template.ParseFiles("index.html"))
	playTemplate = template.Must(template.ParseFiles("play.html"))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

		type mainPage struct {
			Html  template.HTML
			Ms    int
			Games []Game
		}

		data := getMainPageMd()
		html := markdown.ToHTML([]byte(data), nil, nil)
		games := getGameList()
		fmt.Println(games)

		pageData := mainPage{Games: games, Html: template.HTML(html), Ms: 0}

		homeTemplate.Execute(w, pageData)
	})

	http.HandleFunc("/play", playPage)
	http.HandleFunc("/uploadFile", uploadFile)
	http.HandleFunc("/upload", uploadPage)
	http.Handle("/games/", http.StripPrefix("/games/", http.FileServer(http.Dir("./games"))))

	http.ListenAndServe(":1338", nil)
}
