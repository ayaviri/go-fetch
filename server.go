package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ayaviri/goutils/timer"
	"github.com/google/uuid"
	"github.com/gorilla/handlers"
)

var err error
var retailerRegex *regexp.Regexp
var descriptionRegex *regexp.Regexp
var twoDecimalFloatRegex *regexp.Regexp

var db *xDB

func init() {
	// No need to recompile these at every request time
	retailerRegex = regexp.MustCompile("^[\\w\\s&\\-]+$")
	descriptionRegex = regexp.MustCompile("^[\\w\\s\\-]+$")
	twoDecimalFloatRegex = regexp.MustCompile("^\\d+\\.\\d{2}$")
	db = NewXDB()
}

func defineResources() *http.ServeMux {
	logging := newLoggingHandler(os.Stdout)
	var s *http.ServeMux = http.NewServeMux()

	s.Handle("/health", logging(healthHandler()))
	s.Handle("/receipts/", logging(receiptsSubresourceHandler()))

	return s
}

func main() {
	timer.WithTimer("server", func() {
		var s *http.ServeMux = defineResources()
		log.Fatal(http.ListenAndServe(":8000", s))
	})
}

//  ____  _____ ____   ___  _   _ ____   ____ _____
// |  _ \| ____/ ___| / _ \| | | |  _ \ / ___| ____|
// | |_) |  _| \___ \| | | | | | | |_) | |   |  _|
// |  _ <| |___ ___) | |_| | |_| |  _ <| |___| |___
// |_| \_\_____|____/ \___/ \___/|_| \_\\____|_____|
//
//  _   _    _    _   _ ____  _     _____ ____  ____
// | | | |  / \  | \ | |  _ \| |   | ____|  _ \/ ___|
// | |_| | / _ \ |  \| | | | | |   |  _| | |_) \___ \
// |  _  |/ ___ \| |\  | |_| | |___| |___|  _ < ___) |
// |_| |_/_/   \_\_| \_|____/|_____|_____|_| \_\____/
//

func healthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("go fetch !"))
	})
}

func receiptsSubresourceHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Guaranteed to have at least 3 elements, "", "receipts", and ""
		pathSegments := strings.Split(r.URL.Path, "/")

		if len(pathSegments) == 3 && pathSegments[2] == "process" {
			receiptsProcessHandler(w, r)
		} else if len(pathSegments) == 4 && pathSegments[3] == "points" {
			receiptsPointsHandler(w, r)
		}
	})
}

func receiptsProcessHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
		return
	}

	var b ProcessReceiptRequestBody

	timer.WithTimer("reading/unmarshalling request body", func() {
		err = readUnmarshalRequestBody(r, &b)
	})

	if err != nil {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
		return
	}

	var receiptId string

	timer.WithTimer("writing receipt to storage", func() {
		receiptId, err = db.writeReceipt(b.Receipt)
	})

	if err != nil {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
		return
	}

	timer.WithTimer("writing receipt ID to response body", func() {
		responseBody, err := json.Marshal(
			ProcessReceiptsResponseBody{ReceiptId: receiptId},
		)

		if err != nil {
			return
		}

		_, err = w.Write(responseBody)
	})

	if err != nil {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
	}
}

func receiptsPointsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "No receipt found for that ID.", http.StatusNotFound)
		return
	}

	var receiptId string

	timer.WithTimer("getting receipt ID from request URL path", func() {
		receiptId = getReceiptIDFromURLPath(r.URL.Path)
	})

	var receiptPoints int64

	timer.WithTimer("getting the points awarded for the given receipt", func() {
		receiptPoints, err = db.getReceiptPoints(receiptId)
	})

	if err != nil {
		http.Error(w, "No receipt found for that ID.", http.StatusNotFound)
		return
	}

	timer.WithTimer("writing points to response body", func() {
		responseBody, err := json.Marshal(
			ReceiptsPointsResponseBody{Points: receiptPoints},
		)

		if err != nil {
			return
		}

		_, err = w.Write(responseBody)
	})

	if err != nil {
		http.Error(w, "The receipt is invalid.", http.StatusBadRequest)
	}
}

//  ____  _____ ___      ______  _____ ____  ____
// |  _ \| ____/ _ \    / /  _ \| ____/ ___||  _ \
// | |_) |  _|| | | |  / /| |_) |  _| \___ \| |_) |
// |  _ <| |__| |_| | / / |  _ <| |___ ___) |  __/
// |_| \_\_____\__\_\/_/  |_| \_\_____|____/|_|
//
//  ____   ____ _   _ _____ __  __    _    ____
// / ___| / ___| | | | ____|  \/  |  / \  / ___|
// \___ \| |   | |_| |  _| | |\/| | / _ \ \___ \
//  ___) | |___|  _  | |___| |  | |/ ___ \ ___) |
// |____/ \____|_| |_|_____|_|  |_/_/   \_\____/
//

type ProcessReceiptRequestBody struct {
	Receipt
}

type ProcessReceiptsResponseBody struct {
	ReceiptId string `json:"id"`
}

type ReceiptsPointsResponseBody struct {
	Points int64 `json:"points"`
}

//  __  __ ___ ____   ____   ____   ____ _   _ _____ __  __    _    ____
// |  \/  |_ _/ ___| / ___| / ___| / ___| | | | ____|  \/  |  / \  / ___|
// | |\/| || |\___ \| |     \___ \| |   | |_| |  _| | |\/| | / _ \ \___ \
// | |  | || | ___) | |___   ___) | |___|  _  | |___| |  | |/ ___ \ ___) |
// |_|  |_|___|____/ \____| |____/ \____|_| |_|_____|_|  |_/_/   \_\____/
//

type Retailer string

func (r *Retailer) UnmarshalJSON(data []byte) error {
	str, err := obtainQuotedString(&data)

	if err != nil {
		return err
	}

	if !retailerRegex.MatchString(str) {
		return errors.New("Invalid retailer name")
	}

	*r = Retailer(str)
	return nil
}

type Date time.Time

func (d *Date) UnmarshalJSON(data []byte) error {
	str, err := obtainQuotedString(&data)

	if err != nil {
		return err
	}

	var parsedDate time.Time
	parsedDate, err = time.Parse("2006-01-02", str)

	if err != nil {
		return errors.New("Invalid date format")
	}

	*d = Date(parsedDate)
	return nil
}

type Time time.Time

func (t *Time) UnmarshalJSON(data []byte) error {
	str, err := obtainQuotedString(&data)

	if err != nil {
		return err
	}

	var parsedTime time.Time
	parsedTime, err = time.Parse("15:04", str)

	if err != nil {
		return errors.New("Invalid time format")
	}

	*t = Time(parsedTime)
	return nil
}

type Amount float64

func (a *Amount) UnmarshalJSON(data []byte) error {
	str, err := obtainQuotedString(&data)

	if err != nil {
		return err
	}

	if !twoDecimalFloatRegex.MatchString(str) {
		return errors.New("Invalid amount")
	}

	value, err := strconv.ParseFloat(str, 64)

	if err != nil {
		return errors.New("Amount is not a valid float")
	}

	*a = Amount(value)
	return nil
}

type Receipt struct {
	Retailer     Retailer `json:"retailer"`
	PurchaseDate Date     `json:"purchaseDate"`
	PurchaseTime Time     `json:"purchaseTime"`
	Items        []Item   `json:"items"`
	Total        Amount   `json:"total"`
}

func (r *Receipt) computeReceiptPoints() int64 {
	return r.alphanumericRetailerPoints() +
		r.totalRoundDollarAmountPoints() +
		r.totalMultipleOf25CentsPoints() +
		r.every2ItemsPoints() +
		r.itemDescriptionLengthsPoints() +
		r.purchaseDayOddPoints() +
		r.purchaseTimeBetween2And4Points()
}

func (r *Receipt) alphanumericRetailerPoints() int64 {
	var points int64 = 0

	for _, char := range r.Retailer {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			points += 1
		}
	}

	return points
}

func (r *Receipt) totalRoundDollarAmountPoints() int64 {
	if float64(r.Total) == math.Trunc(float64(r.Total)) {
		return 50
	} else {
		return 0
	}
}

func (r *Receipt) totalMultipleOf25CentsPoints() int64 {
	if math.Abs(math.Mod(float64(r.Total), 0.25)) < 1e-4 {
		return 25
	} else {
		return 0
	}
}

func (r *Receipt) every2ItemsPoints() int64 {
	return int64(5 * (len(r.Items) / 2))
}

func (r *Receipt) itemDescriptionLengthsPoints() int64 {
	var points int64 = 0

	for _, item := range r.Items {
		trimmedDescription := strings.TrimSpace(string(item.Description))
		if len(trimmedDescription)%3 == 0 {
			points += int64(math.Ceil(float64(item.Price) * 0.2))
		}
	}

	return points
}

func (r *Receipt) purchaseDayOddPoints() int64 {
	if time.Time(r.PurchaseDate).Day()%2 == 1 {
		return 6
	} else {
		return 0
	}
}

func (r *Receipt) purchaseTimeBetween2And4Points() int64 {
	purchaseHour := time.Time(r.PurchaseTime).Hour()

	if purchaseHour >= 14 && purchaseHour < 16 {
		return 10
	} else {
		return 0
	}
}

type Description string

func (d *Description) UnmarshalJSON(data []byte) error {
	str, err := obtainQuotedString(&data)

	if err != nil {
		return err
	}

	if !descriptionRegex.MatchString(str) {
		return errors.New("Invalid item description")
	}

	*d = Description(str)
	return nil
}

type Item struct {
	Description Description `json:"shortDescription"`
	Price       Amount      `json:"price"`
}

//  __  __ ___ ____  ____  _     _______        ___    ____  _____
// |  \/  |_ _|  _ \|  _ \| |   | ____\ \      / / \  |  _ \| ____|
// | |\/| || || | | | | | | |   |  _|  \ \ /\ / / _ \ | |_) |  _|
// | |  | || || |_| | |_| | |___| |___  \ V  V / ___ \|  _ <| |___
// |_|  |_|___|____/|____/|_____|_____|  \_/\_/_/   \_\_| \_\_____|
//

func newLoggingHandler(destination io.Writer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return handlers.LoggingHandler(destination, next)
	}
}

//  __  __ ___ ____   ____   _   _ _____ ___ _     ___ _____ ___ _____ ____
// |  \/  |_ _/ ___| / ___| | | | |_   _|_ _| |   |_ _|_   _|_ _| ____/ ___|
// | |\/| || |\___ \| |     | | | | | |  | || |    | |  | |  | ||  _| \___ \
// | |  | || | ___) | |___  | |_| | | |  | || |___ | |  | |  | || |___ ___) |
// |_|  |_|___|____/ \____|  \___/  |_| |___|_____|___| |_| |___|_____|____/
//

// Reads the entirety of the given request's body and unmarshalls it into
// the given pointer to the JSON schema
func readUnmarshalRequestBody(request *http.Request, schema any) error {
	var requestBodyBytes []byte
	requestBodyBytes, err = io.ReadAll(request.Body)

	if err != nil {
		return err
	}

	err = json.Unmarshal(requestBodyBytes, schema)

	if err != nil {
		return err
	}

	return nil
}

// This path has already been validated as having the format
// "/receipts/foo/points"
func getReceiptIDFromURLPath(path string) string {
	pathSegments := strings.Split(path, "/")

	return pathSegments[2]
}

// Returns true if the length of the given string is at least 2 and
// it is wrapped in double quotes
func isQuotedString(s string) bool {
	return len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"'
}

// 1) Converts the byte slice to a string
// 2) Checks if the string is surrounded by double quotes
// 3) Returns the substring in between the double quotes
func obtainQuotedString(data *[]byte) (string, error) {
	str := string(*data)

	if !isQuotedString(str) {
		return "", errors.New("Field must be a quoted string")
	}

	return str[1 : len(str)-1], nil
}

//  _ _ ____  ____ _ _
// ( | )  _ \| __ | | )
//  V V| | | |  _ \V V
//     | |_| | |_) |
//     |____/|____/
//

type xDB struct {
	Data map[string]any
	Mu   sync.RWMutex
}

func NewXDB() *xDB {
	return &xDB{
		Data: make(map[string]any),
	}
}

type ReceiptRow struct {
	Receipt
	ReceiptId string
	Points    int64
	// TODO: A CreationDate field here might be nice
}

const ReceiptTableName = "receipt"

func (db *xDB) writeReceipt(r Receipt) (string, error) {
	receiptId := uuid.NewString()
	row := ReceiptRow{
		Receipt:   r,
		ReceiptId: receiptId,
		Points:    r.computeReceiptPoints(),
	}

	db.Mu.Lock()
	defer db.Mu.Unlock()

	db.Data[ReceiptTableName+"."+receiptId] = row

	return receiptId, nil
}

func (db *xDB) getReceiptPoints(receiptId string) (int64, error) {
	db.Mu.RLock()
	defer db.Mu.RUnlock()

	key := ReceiptTableName + "." + receiptId

	if value, exists := db.Data[key]; exists {
		// Casting here, I never really liked the syntax for it in Go
		receiptRow, ok := value.(ReceiptRow)

		if ok {
			return receiptRow.Points, nil
		}

		return 0, errors.New("Receipt with given ID was malformed")
	}

	return 0, errors.New("No receipt with given ID exists")
}
