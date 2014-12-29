package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/minotar/minecraft"
	"github.com/stathat/go"
	"net/http"
	"strconv"
	"strings"
)

type Router struct {
	Mux *mux.Router
}

type NotFoundHandler struct{}

// Handles 404 errors
func (h NotFoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(404)
	fmt.Fprintf(w, "404 not found")
}

// Converts and sanitizes the string for the avatar size.
func (r *Router) GetSize(inp string) uint {
	out64, err := strconv.ParseUint(inp, 10, 0)
	out := uint(out64)
	if err != nil {
		return DefaultSize
	} else if out > MaxSize {
		return MaxSize
	} else if out < MinSize {
		return MinSize
	}
	return out

}

// Shows only the user's skin.
func (router *Router) SkinPage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	username := vars["username"]

	skin := fetchSkin(username)

	w.Header().Add("Content-Type", "image/png")
	w.Header().Add("X-Requested", "skin")
	w.Header().Add("X-Result", "ok")

	skin.WriteSkin(w)
}

// Shows the skin and tells the browser to attempt to download it.
func (router *Router) DownloadPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Disposition", "attachment; filename=\"skin.png\"")
	router.SkinPage(w, r)
}

// Pull the Get<resource> method from the skin. Originally this used
// reflection, but that was slow.
func (router *Router) ResolveMethod(skin *mcSkin, resource string) func(int) error {
	stats.Served(resource)

	switch resource {
	case "Avatar":
		stathat.PostEZCountOne("served-avatar", config.Server.StatHat)
		return skin.GetHead
	case "Helm":
		stathat.PostEZCountOne("served-helm", config.Server.StatHat)
		return skin.GetHelm
	case "Cube":
		stathat.PostEZCountOne("served-cube", config.Server.StatHat)
		return skin.GetCube
	case "Bust":
		stathat.PostEZCountOne("served-bust", config.Server.StatHat)
		return skin.GetBust
	case "Body":
		stathat.PostEZCountOne("served-body", config.Server.StatHat)
		return skin.GetBody
	case "Armor/Bust":
		stathat.PostEZCountOne("served-armor/bust", config.Server.StatHat)
		return skin.GetArmorBust
	case "Armour/Bust":
		stathat.PostEZCountOne("served-armor/bust", config.Server.StatHat)
		return skin.GetArmorBust
	case "Armor/Body":
		stathat.PostEZCountOne("served-armor/body", config.Server.StatHat)
		return skin.GetArmorBody
	case "Armour/Body":
		stathat.PostEZCountOne("served-armor/body", config.Server.StatHat)
		return skin.GetArmorBody
	default:
		stathat.PostEZCountOne("served-default", config.Server.StatHat)
		return skin.GetHelm
	}
}

// Binds the route and makes a handler function for the requested resource.
func (router *Router) Serve(resource string) {
	fn := func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		size := router.GetSize(vars["size"])
		skin := fetchSkin(vars["username"])

		if r.Header.Get("If-None-Match") == skin.Skin.Hash {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		err := router.ResolveMethod(skin, resource)(int(size))
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "500 internal server error")
			return
		}

		log.Info(r.RemoteAddr + "|" + r.RequestURI + "|" + r.Host + "|" + r.Header.Get("Origin") + "|" + r.Header.Get("Via") + "|" + r.Header.Get("Referer"))
		w.Header().Add("Content-Type", "image/png")
		w.Header().Add("Cache-Control", fmt.Sprintf("public, max-age=%d", TimeoutActualSkin))
		w.Header().Add("ETag", skin.Hash)
		skin.WritePNG(w)
	}

	router.Mux.HandleFunc("/"+strings.ToLower(resource)+"/{username:"+minecraft.ValidUsernameRegex+"}{extension:(.png)?}", fn)
	router.Mux.HandleFunc("/"+strings.ToLower(resource)+"/{username:"+minecraft.ValidUsernameRegex+"}/{size:[0-9]+}{extension:(.png)?}", fn)
}

// Binds routes to the ServerMux.
func (router *Router) Bind() {

	router.Mux.NotFoundHandler = NotFoundHandler{}

	router.Serve("Avatar")
	router.Serve("Helm")
	router.Serve("Cube")
	router.Serve("Bust")
	router.Serve("Body")
	router.Serve("Armor/Bust")
	router.Serve("Armour/Bust")
	router.Serve("Armor/Body")
	router.Serve("Armour/Body")

	router.Mux.HandleFunc("/download/{username:"+minecraft.ValidUsernameRegex+"}{extension:(.png)?}", router.DownloadPage)
	router.Mux.HandleFunc("/skin/{username:"+minecraft.ValidUsernameRegex+"}{extension:(.png)?}", router.SkinPage)

	router.Mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", MinotarVersion)
	})

	router.Mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(stats.ToJSON())
	})

	router.Mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://mcme.co/", 302)
	})
}

func fetchSkin(username string) *mcSkin {
	if cache.has(strings.ToLower(username)) {
		stats.HitCache()
		return &mcSkin{Processed: nil, Skin: cache.pull(strings.ToLower(username))}
	}

	skin, err := minecraft.FetchSkinFromMojang(username)
	if err != nil {
		log.Error("Failed Skin Mojang: " + username + " (" + err.Error() + ")")
		// Let's fallback to S3 and try and serve at least an old skin...
		skin, err = minecraft.FetchSkinFromS3(username)
		if err != nil {
			log.Error("Failed Skin S3: " + username + " (" + err.Error() + ")")
			// Well, looks like they don't exist after all.
			skin, _ = minecraft.FetchSkinForChar()
		}
	}

	stats.MissCache()
	cache.add(strings.ToLower(username), skin)

	return &mcSkin{Processed: nil, Skin: skin}
}
