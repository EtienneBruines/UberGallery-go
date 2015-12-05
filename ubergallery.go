package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"bitbucket.org/huperwebs/webutils/templates"
	"fmt"
	"github.com/go-ini/ini"
	"github.com/julienschmidt/httprouter"
	"github.com/nfnt/resize"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
)

const (
	ConfigFilename   = "galleryConfig.ini"
	GalleryDirectory = "gallery-images"
	ViewDirectory    = "view"

	Port = 8080
)

var (
	router = httprouter.New()
	config *Config
)

type Config struct {
	CacheExpiration int

	EnablePagination   bool
	PaginatorThreshold int
	ImagesPerPage      int

	ThumbnailWidth   uint
	ThumbnailHeight  uint
	ThumbnailQuality int
	ThemeName        string

	ImageSortBy string
	ReverseSort bool

	EnableDebugging bool
}

func ReadConfig(filename string) (*Config, error) {
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	cfg, err := ini.Load(contents)
	if err != nil {
		return nil, err
	}

	config := &Config{}

	for _, section := range cfg.Sections() {
		switch section.Name() {
		case "DEFAULT":
			// Nothing to do here
		case "basic_settings":
			for _, key := range section.Keys() {
				switch key.Name() {
				case "cache_expiration":
					i, err := strconv.Atoi(key.Value())
					if err != nil {
						log.Println("Error: 'cache_expiration' unable to parse integer:", err)
					}
					config.PaginatorThreshold = i
				case "enable_pagination":
					config.EnablePagination = (key.Value() == "true")
				case "paginator_threshold":
					i, err := strconv.Atoi(key.Value())
					if err != nil {
						log.Println("Error: 'paginator_threshold' unable to parse integer:", err)
					}
					config.PaginatorThreshold = i
				case "thumbnail_width":
					i, err := strconv.Atoi(key.Value())
					if err != nil {
						log.Println("Error: 'thumbnail_width' unable to parse integer:", err)
					}
					config.ThumbnailWidth = uint(i)
				case "thumbnail_height":
					i, err := strconv.Atoi(key.Value())
					if err != nil {
						log.Println("Error: 'thumbnail_height' unable to parse integer:", err)
					}
					config.ThumbnailHeight = uint(i)
				case "thumbnail_quality":
					i, err := strconv.Atoi(key.Value())
					if err != nil {
						log.Println("Error: 'thumbnail_quality' unable to parse integer:", err)
					}
					config.ThumbnailQuality = i
					if i < 0 {
						log.Println("Warning: 'thumbnail_quality' must be >= 0; set to 0")
						config.ThumbnailQuality = 0
					}
					if i > 100 {
						log.Println("Warning: 'thumbnail_quality' must be <= 100; set to 100")
						config.ThumbnailQuality = 100
					}
				case "theme_name":
					config.ThemeName = key.Value()
				default:
					log.Println("Warning: unsupported key:", key.Name())
				}
			}
		case "advanced_settings":
			for _, key := range section.Keys() {
				switch key.Name() {
				case "images_per_page":
					i, err := strconv.Atoi(key.Value())
					if err != nil {
						log.Println("Error: 'images_per_page' unable to parse integer:", err)
					}
					config.ImagesPerPage = i
				case "images_sort_by":
				case "reverse_sort":
					config.ReverseSort = (key.Value() == "true")
				case "enable_debugging":
					config.EnableDebugging = (key.Value() == "true")
				default:
					log.Println("Warning: unsupported key:", key.Name())
				}
			}
		default:
			log.Println("Warning: unsupported section:", section.Name())
		}
	}

	return config, nil
}

type Image struct {
	Name      string
	Thumbnail string
	URL       string
}

func DefaultRoute(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	var images []Image

	// Load images
	files, err := ioutil.ReadDir(filepath.Join("public", GalleryDirectory))
	if err != nil {
		templates.WriteInternalError(w, err.Error())
		return
	}

	for _, file := range files {
		if !file.IsDir() {
			// Generate thumbnail if needed

			images = append(images, Image{
				Name:      file.Name(),
				Thumbnail: GenerateThumbnail(file.Name()),
				URL:       "/" + filepath.Join("public", GalleryDirectory, file.Name()),
			})
		}
	}

	p := templates.NewPage("images", images)
	templates.Execute(w, r, config.ThemeName+".html", p)
}

func GenerateThumbnail(filename string) string {
	thumbName := fmt.Sprintf("%s/%dx%d-%s",
		filepath.Join("public", "cache"),
		config.ThumbnailWidth,
		config.ThumbnailHeight,
		filename,
	)

	// Generate it if needed
	if _, err := os.Stat(thumbName); os.IsNotExist(err) {
		file, err := os.Open(filepath.Join("public", GalleryDirectory, filename))
		if err != nil {
			log.Println("Warning: could not create thumbnail for", filename, err)
			return filepath.Join("public", GalleryDirectory, filename)
		}

		// Load original file
		img, _, err := image.Decode(file)
		if err != nil {
			log.Println("Warning: could not create thumbnail for", filename, err)
			return filepath.Join("public", GalleryDirectory, filename)
		}

		// Resize and save
		thumb := resize.Thumbnail(config.ThumbnailWidth, config.ThumbnailHeight, img, resize.Lanczos2)

		thumbFile, err := os.Create(thumbName)
		if err != nil {
			log.Println("Warning: could not create thumbnail for", filename, err)
			return filepath.Join("public", GalleryDirectory, filename)
		}
		jpeg.Encode(thumbFile, thumb, &jpeg.Options{config.ThumbnailQuality})
		thumbFile.Close()
	}

	return thumbName
}

func LoadViews() {
	templates.Init(&templates.Config{
		ProjectView: ViewDirectory,
		Handler:     nil,
	})
	templates.PreloadTemplate(config.ThemeName + ".html")
	templates.PreloadTemplates()
}

func main() {
	var err error

	// Read config
	config, err = ReadConfig(ConfigFilename)
	if err != nil {
		log.Fatal(err)
	}

	// Load HTML views
	LoadViews()

	// TODO: listen for "reload" signal

	// Register route
	staticHandler := http.FileServer(http.Dir("./"))
	router.GET("/public/*filepath", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		staticHandler.ServeHTTP(w, r)
	})
	router.GET("/", DefaultRoute)

	log.Println("Notice: server started listening at port", Port, "...")
	err = http.ListenAndServe(":"+strconv.Itoa(Port), router)
	if err != nil {
		log.Fatal("Error: server shut down:", err)
	}
}
