package pokervision

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"reflect"
	"testing"
)

func TestNewMatcher(t *testing.T) {
	type args struct {
		refFile string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"Valid", args{"./testdata/refs.json"}, false},
		{"Does not exist", args{"./testdata/noExist.json"}, true},
		{"Malformed", args{"./testdata/malformed.json"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMatcher(tt.args.refFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMatcherFromFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_matcher_Match(t *testing.T) {

	const refFile = "./testdata/refs.json"

	img, err := loadImage("./testdata/master.png")
	if err != nil {
		t.Errorf("matcher.Match() failed to load master image. %v", err)
	}

	m, err := NewMatcher(refFile)
	if err != nil {
		t.Errorf("matcher.Match() failed to load ref file. %v", err)
	}

	type args struct {
		srcName string
		img     image.Image
	}
	tests := []struct {
		name    string
		args    args
		wantRef string
	}{
		{"Image match", args{"srcImg1", img}, "refImg2"},
		{"Image no match", args{"srcImg2", img}, ""},
		{"Image monochrome match", args{"srcMImg1", img}, "refMImg1"},
		{"Image monochrome no match", args{"srcMImg2", img}, ""},
		{"OCR", args{"srcOCR", img}, "runnings"},
		{"Color match", args{"srcColor1", img}, "refColor2"},
		{"Color no match", args{"srcColor2", img}, ""},
		{"Invalid source #1", args{"invalidSrc1", img}, ""},
		{"Invalid source #2", args{"invalidSrc2", img}, ""},
		{"Invalid image source", args{"invalidImageSrc", img}, ""},
		{"Invalid color source #1", args{"invalidColorSrc1", img}, ""},
		{"Invalid color source #2", args{"invalidColorSrc2", img}, ""},
		{"No source", args{"noSuchSource", img}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if gotRef := m.Match(tt.args.srcName, tt.args.img); gotRef != tt.wantRef {
				t.Errorf("matcher.Match() = %v, want %v", gotRef, tt.wantRef)
			}
		})
	}
}

func Test_matcher_findSource(t *testing.T) {
	type fields struct {
		Srcs []source
		Refs []reference
	}

	s1 := source{"source1", nil, nil}
	s2 := source{"source2", nil, nil}

	type args struct {
		srcName string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *source
	}{
		{"Exists", fields{[]source{s1, s2}, nil}, args{"source2"}, &s2},
		{"Does not exist", fields{[]source{s1, s2}, nil}, args{"source3"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			im := &matcher{
				Srcs: tt.fields.Srcs,
				Refs: tt.fields.Refs,
			}
			if got := im.findSource(tt.args.srcName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("matcher.findSource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_handleImage(t *testing.T) {

	ref1 := &reference{Name: "name1", Ref: "color:#FFFFFF"}
	ref2 := &reference{Name: "name2", Ref: "image:./testdata/blackVal.png"}
	ref3 := &reference{Name: "name3", Ref: "imageM:./testdata/redVal.png"}

	img1, err := loadImage("./testdata/blackVal.png")
	if err != nil {
		t.Errorf("handleImage() failed to load test files. %v", err)
	}

	type args struct {
		r      *reference
		srcImg image.Image
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"Invalid reference", args{ref1, img1}, ""},
		{"Match image", args{ref2, img1}, "name2"},
		{"Match monochrome image", args{ref3, img1}, "name3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handleImage(tt.args.r, tt.args.srcImg); got != tt.want {
				t.Errorf("handleImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_handleColor(t *testing.T) {

	ref1 := &reference{Name: "name1", Ref: "color:#FFFFFF"}
	ref2 := &reference{Name: "name2", Ref: "color:#FFFFFE"}
	ref3 := &reference{Name: "name3", Ref: "color:#42f44e"}
	ref4 := &reference{Name: "name4", Ref: "color:#4268f4"}
	ref5 := &reference{Name: "name5", Ref: "color:#4268f47"}
	ref6 := &reference{Name: "name5", Ref: "color:#4268fg"}

	col1 := color.RGBA{255, 255, 255, 0}
	col2 := color.RGBA{66, 244, 78, 0}

	type args struct {
		r        *reference
		srcColor color.Color
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"White match", args{ref1, col1}, "name1"},
		{"White no match", args{ref2, col1}, ""},
		{"Color match", args{ref3, col2}, "name3"},
		{"Color no match", args{ref4, col2}, ""},
		{"Invalid color #1", args{ref5, col2}, ""},
		{"Invalid color #2", args{ref6, col2}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handleColor(tt.args.r, tt.args.srcColor); got != tt.want {
				t.Errorf("handleColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_handleOCR(t *testing.T) {

	light1, err := loadImage("./testdata/lightText1.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}
	light2, err := loadImage("./testdata/lightText2.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}
	dark1, err := loadImage("./testdata/darkText1.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}
	dark2, err := loadImage("./testdata/darkText2.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}
	lightNum1, err := loadImage("./testdata/lightNum1.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}
	lightNum2, err := loadImage("./testdata/lightNum2.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}
	darkNum1, err := loadImage("./testdata/darkNum1.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}
	darkNum2, err := loadImage("./testdata/darkNum2.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}
	pot, err := loadImage("./testdata/pot.png")
	if err != nil {
		t.Errorf("handleOCR() failed to load test files. %v", err)
	}

	type args struct {
		srcImg image.Image
		args   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"Light #1", args{light1, "200,y"}, "skendroshen"},
		{"Light #2", args{light2, "200,y"}, "runnings"},
		{"Dark #1", args{dark1, "200,Y"}, "luistirelli"},
		{"Dark #2", args{dark2, "200,Y"}, "boasss"},
		{"Light number #1", args{lightNum1, "200,n"}, "$1.98"},
		{"Light number #2", args{lightNum2, "200,n"}, "$2.66"},
		{"Dark number #1", args{darkNum1, "200,n"}, "$0.98"},
		{"Dark number #2", args{darkNum2, "200,n"}, "$2.39"},
		{"Pot", args{pot, "200,n"}, "$0.03"},
		{"Invalid arg", args{dark2, "asd"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handleOCR(tt.args.srcImg, tt.args.args); got != tt.want {
				t.Errorf("handleOCR() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_compareImages(t *testing.T) {

	img1, err := loadImage("./testdata/redVal.png")
	if err != nil {
		t.Errorf("compareImages() failed to load test files. %v", err)
	}
	img2, err := loadImage("./testdata/blackVal.png")
	if err != nil {
		t.Errorf("compareImages() failed to load test files. %v", err)
	}
	img3, err := loadImage("./testdata/blackValCropped.png")
	if err != nil {
		t.Errorf("compareImages() failed to load test files. %v", err)
	}

	type args struct {
		img1 image.Image
		img2 image.Image
	}
	tests := []struct {
		name      string
		args      args
		wantEqual bool
	}{
		{"Identical", args{img1, img1}, true},
		{"Different", args{img1, img2}, false},
		{"Different size", args{img2, img3}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotEqual := compareImages(tt.args.img1, tt.args.img2); gotEqual != tt.wantEqual {
				t.Errorf("compareImages() = %v, want %v", gotEqual, tt.wantEqual)
			}
		})
	}
}

func Test_compareImagesMonochrome(t *testing.T) {

	img1, err := loadImage("./testdata/redVal.png")
	if err != nil {
		t.Errorf("compareImagesMonochrome() failed to load test files. %v", err)
	}
	img2, err := loadImage("./testdata/blackVal.png")
	if err != nil {
		t.Errorf("compareImagesMonochrome() failed to load test files. %v", err)
	}
	img3, err := loadImage("./testdata/blackValCropped.png")
	if err != nil {
		t.Errorf("compareImagesMonochrome() failed to load test files. %v", err)
	}
	img4, err := loadImage("./testdata/blackValModified.png")
	if err != nil {
		t.Errorf("compareImagesMonochrome() failed to load test files. %v", err)
	}

	type args struct {
		img1 image.Image
		img2 image.Image
	}
	tests := []struct {
		name      string
		args      args
		wantEqual bool
	}{
		{"Identical", args{img1, img1}, true},
		{"Monochrome identical", args{img1, img2}, true},
		{"Monochrome identical opposite", args{img2, img1}, true},
		{"Different size", args{img1, img3}, false},
		{"Modified", args{img1, img4}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotEqual := compareImagesMonochrome(tt.args.img1, tt.args.img2); gotEqual != tt.wantEqual {
				t.Errorf("compareImagesMonochrome() = %v, want %v", gotEqual, tt.wantEqual)
			}
		})
	}
}

func Test_loadImage(t *testing.T) {

	type args struct {
		fileName string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"Valid file", args{"./testdata/lightText1.png"}, false},
		{"Invalid file", args{"./testdata/invalidFile.png"}, true},
		{"Invalid file type", args{"./testdata/invalidFile"}, true},
		{"Does not exit", args{"./testdata/doesNotExist.png"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadImage(tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func Test_defaultFileLoader_Load(t *testing.T) {
	type args struct {
		fileName string
	}
	tests := []struct {
		name string
		l    *defaultFileLoader
		args args
		want io.Reader
	}{
	// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &defaultFileLoader{}
			if got := l.Load(tt.args.fileName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("defaultFileLoader.Load() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testLoader struct{}

func (*testLoader) Load(s string) io.Reader {
	switch s {
	case "1":
		return nil
	case "2":
		return bytes.NewReader(nil)
	}
	return nil
}

func TestSetFileLoader(t *testing.T) {

	SetFileLoader(&testLoader{})

	if r := fileLoader.Load("1"); r != nil {
		t.Errorf("TestSetFileLoader() = %v, want nil", r)
	}
	if r := fileLoader.Load("2"); r == nil {
		t.Errorf("TestSetFileLoader() = %v, want io.Reader", r)
	}
}
