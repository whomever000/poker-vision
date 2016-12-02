package pokervision

import (
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/nfnt/resize"
	"github.com/otiai10/gosseract"
)

// Matcher is the public interface to a matcher.
type Matcher interface {
	Match(srcName string, img image.Image) string
}

// NewMatcher creates a new matcher from a JSON ref file.
func NewMatcher(refFile string) (Matcher, error) {

	// Read JSON file containing references.
	buf, err := ioutil.ReadFile(refFile)
	if err != nil {
		return nil, err
	}

	// Fill data from JSON into matcher.
	var m matcher
	err = json.Unmarshal(buf, &m)
	if err != nil {
		return nil, err
	}

	return &m, nil
}

// subImager provides an interface for image-types with the SubImage() function.
type subImager interface {
	image.Image
	SubImage(r image.Rectangle) image.Image
}

// source describes a rectangle or point on the sceen that should be sampled.
type source struct {
	Name string
	Src  []int
	Refs []string
}

// reference describes a reference color or image to be compared against.
type reference struct {
	Name string
	Ref  string
}

// matcher allows for finding color or image matches. The comparisons are
// described by the JSON format (same name).
type matcher struct {
	Srcs []source
	Refs []reference
}

// Match matches a source (specified by srcName) with its assiocitated references.
func (im *matcher) Match(srcName string, img image.Image) (ref string) {

	var isPixel bool
	var srcImg image.Image
	var srcColor color.Color

	// Locate source
	s := im.findSource(srcName)
	if s == nil {
		log.Printf("warning: source does not exist srcName=%v", srcName)
		return ""
	}

	// Grap pixels/image from source.
	switch len(s.Src) {

	// Pixel (described by 2 ints).
	case 2:

		// Grab pixel.
		srcColor = img.At(s.Src[0], s.Src[1])
		isPixel = true

	// Image (described by 4 ints).
	case 4:

		rect := image.Rect(
			s.Src[0],          // X
			s.Src[1],          // Y
			s.Src[0]+s.Src[2], // X+width
			s.Src[1]+s.Src[3]) // Y+height

		// Grab subimage.
		srcImg = img.(subImager).SubImage(rect)
		isPixel = false

	default:
		log.Printf(`error: illegal source - len(Src) must be 2 or 4 srcName=%v`,
			srcName)
		return ""
	}

	// Compare against each reference.
	for _, r := range im.Refs {

		// Determine if this ref should be considered.
		skip := true
		for _, rName := range s.Refs {

			// The source is referring to this reference.
			if r.Name == rName {

				// Stop looking.
				skip = false
				break
			}
		}
		// This reference should NOT be considered.
		if skip {
			continue
		}

		// Handle color.
		if strings.HasPrefix(r.Ref, "color:") {
			// Color cannot be compared against image.
			if !isPixel {
				log.Printf(`error: Cannot compare image against color srcName=%v
				refName=%v`, srcName, r.Name)
				return ""
			}

			match := handleColor(&r, srcColor)
			if len(match) != 0 {
				return match
			}

			// Handle OCR.
		} else if strings.HasPrefix(r.Ref, "ocr:") {

			// Image cannot be compared against pixel.
			if isPixel {
				log.Printf(`error: Cannot do OCR on pixel srcName=%v
				refName=%v`, srcName, r.Name)
				return ""
			}

			args := ""
			if len(r.Ref) > 4 {
				args = r.Ref[4:]
			}

			match := handleOCR(srcImg, args)

			if len(match) != 0 {
				return match
			}

			// Handle Image (monochrome or not).
		} else if strings.HasPrefix(r.Ref, "image") {

			// Image cannot be compared against pixel.
			if isPixel {
				log.Printf(`error: Cannot compare pixel against image srcName=%v
				refName=%v`, srcName, r.Name)
				return ""
			}

			match := handleImage(&r, srcImg)
			if len(match) != 0 {
				return match
			}
		} else {
			log.Printf("error: Invalid reference type refName=%v ref=%v",
				r.Name, r.Ref)
			return ""
		}
	}

	// No match found.
	return ""

}

// findSource finds a source given its name.
func (im *matcher) findSource(srcName string) *source {
	for _, s := range im.Srcs {
		if s.Name == srcName {
			return &s
		}
	}

	return nil
}

// handleImage handles a comparison with a image (monochrome or not).
func handleImage(r *reference, srcImg image.Image) string {

	var file string

	// Get filename from ref string.
	if strings.HasPrefix(r.Ref, "image:") {

		file = r.Ref[len("image:"):]

	} else if strings.HasPrefix(r.Ref, "imageM:") {

		file = r.Ref[len("imageM:"):]

	} else {

		log.Printf("error: Illegal image type refName=%v ref=%v", r.Name, r.Ref)

	}

	// Load reference image.
	refImg, err := loadImage(file)
	if err != nil {
		log.Printf("error: %v refName='%v'", err, r.Name)
		return ""
	}

	// Compare the images.
	if strings.HasPrefix(r.Ref, "imageM:") {

		// Monochrome comparison.
		if compareImagesMonochrome(refImg, srcImg) {

			// Match.
			return r.Name
		}

	} else {

		// Normal comparison.
		if compareImages(refImg, srcImg) {

			// Match.
			return r.Name
		}
	}

	// No match.
	return ""
}

// handleColor handles a comparison with a color reference.
func handleColor(r *reference, srcColor color.Color) string {
	const preLen = len("color:")

	// Assert HTML color format (this check allows the following slicing).
	if len(r.Ref) != (preLen + 7) {
		log.Printf(`error: invalid color, expected HTML color
				refName=%v color=%v`, r.Name, r.Ref)
		return ""
	}

	b, err := hex.DecodeString(r.Ref[preLen+1:])
	if err != nil {
		log.Printf(`error: invalid color, expected HTML color
				refName=%v color=%v`, r.Name, r.Ref)
		return ""
	}

	// Compare colors.
	red, green, blue, _ := srcColor.RGBA()

	if (red/256) == uint32(b[0]) &&
		(green/256) == uint32(b[1]) &&
		(blue/256) == uint32(b[2]) {
		// Match.
		return r.Name
	}

	// No match.
	return ""
}

// handleOCR handles a OCR operation
func handleOCR(srcImg image.Image, args string) string {

	strs := strings.Split(args, ",")
	for i, arg := range strs {
		switch i {

		// Image width.
		case 0:
			w, err := strconv.Atoi(arg)
			if err != nil {
				log.Printf("error: Illegal OCR arg width=%v", arg)
				return ""
			}

			if w > 0 {
				srcImg = resize.Resize(uint(w), 0, srcImg, resize.Bilinear)
			}

		}
	}

	client, _ := gosseract.NewClient()
	out, _ := client.Image(srcImg).Out()

	// LEET-ify numbers
	out = strings.Replace(out, "1", "l", -1)
	out = strings.Replace(out, "2", "r", -1)
	out = strings.Replace(out, "3", "e", -1)
	out = strings.Replace(out, "4", "a", -1)
	out = strings.Replace(out, "5", "s", -1)
	out = strings.Replace(out, "6", "g", -1)
	out = strings.Replace(out, "7", "t", -1)
	out = strings.Replace(out, "8", "b", -1)
	out = strings.Replace(out, "9", "g", -1)

	regx := regexp.MustCompile("[ \\n]")
	out = regx.ReplaceAllString(out, "")
	return strings.ToLower(out)
}

// compareImages compares two images pixel by pixel. Images must be of same size
// and have identical values for all pixel in order for function to return true.
func compareImages(img1 image.Image, img2 image.Image) (equal bool) {

	// Make sure dimensions are equal.
	if img1.Bounds().Dx() != img2.Bounds().Dx() ||
		img1.Bounds().Dy() != img2.Bounds().Dy() {
		return false
	}

	// Get offsets.
	sx1 := img1.Bounds().Min.X
	sx2 := img2.Bounds().Min.X
	sy1 := img1.Bounds().Min.Y
	sy2 := img2.Bounds().Min.Y

	size := img1.Bounds().Size()
	var r1 uint32
	var g1 uint32
	var b1 uint32

	var r2 uint32
	var g2 uint32
	var b2 uint32

	// Compare pixels.
	for x := 0; x < size.X; x++ {
		for y := 0; y < size.Y; y++ {
			r1, g1, b1, _ = img1.At(x+sx1, y+sy1).RGBA()
			r2, g2, b2, _ = img2.At(x+sx2, y+sy2).RGBA()

			if r1 != r2 || g1 != g2 || b1 != b2 {
				return false
			}
		}
	}

	return true
}

// compareImagesMonochrome compares two images pixel by pixel after clamping
// to colors. Colors are differentiated between white and non-white colors.
// Images must be of same size and have identical values for all pixel in order
// for function to return true.
func compareImagesMonochrome(img1 image.Image, img2 image.Image) (equal bool) {

	// Make sure dimensions are equal.
	if img1.Bounds().Dx() != img2.Bounds().Dx() ||
		img1.Bounds().Dy() != img2.Bounds().Dy() {
		log.Printf("warning: images are not of the same size img1='%v,%v' img2='%v,%v'",
			img1.Bounds().Dx(), img1.Bounds().Dy(),
			img2.Bounds().Dx(), img2.Bounds().Dy())
		return false
	}

	// Get offsets.
	sx1 := img1.Bounds().Min.X
	sx2 := img2.Bounds().Min.X
	sy1 := img1.Bounds().Min.Y
	sy2 := img2.Bounds().Min.Y

	size := img1.Bounds().Size()
	var r1 uint32
	var g1 uint32
	var b1 uint32

	var r2 uint32
	var g2 uint32
	var b2 uint32

	var img1White bool
	var img2White bool

	// Compare pixels.
	for x := 0; x < size.X; x++ {
		for y := 0; y < size.Y; y++ {
			r1, g1, b1, _ = img1.At(x+sx1, y+sy1).RGBA()
			r2, g2, b2, _ = img2.At(x+sx2, y+sy2).RGBA()

			img1White = (r1 == 65535 && g1 == 65535 && b1 == 65535)
			img2White = (r2 == 65535 && g2 == 65535 && b2 == 65535)

			if img1White != img2White {
				return false
			}
		}
	}

	return true
}

// loadImage loads and png image.
func loadImage(fileName string) (refImg image.Image, err error) {

	// Load image file.
	imgFile, err := os.Open(fileName)
	if err != nil {
		log.Printf("error: %v", err)
		return
	}
	defer imgFile.Close()

	// Decode image.
	refImg, err = png.Decode(imgFile)
	if err != nil {
		log.Printf("error: %v", err)
	}

	return
}
