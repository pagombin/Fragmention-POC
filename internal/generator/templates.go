package generator

import (
	"math/rand"

	"github.com/brianvoe/gofakeit/v7"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// DefaultTemplates returns the spec § 5.1 template set with reasonable
// weights for a varied workload. Callers may override weights per run.
func DefaultTemplates() ([]Template, []float64) {
	return []Template{
			&UserProfile{},
			&EventLog{},
			&Order{},
			&Telemetry{},
			&DocumentBlob{},
			&IoTTimeseries{},
		},
		// Weights favor small, high-volume docs over large ones so we fill
		// collections with realistic row counts. Blob collections still get
		// a small share so we exercise >50KB WiredTiger page behavior.
		[]float64{3, 6, 2, 6, 1, 3}
}

// UserProfile models a typical user document (~1-2KB).
type UserProfile struct{}

// Name implements Template.
func (UserProfile) Name() string { return "user_profile" }

// IDKind implements Template.
func (UserProfile) IDKind() IDKind { return IDKindObjectID }

// IndexSpecs implements Template.
func (UserProfile) IndexSpecs() []IndexSpec {
	return []IndexSpec{
		{Name: "idx_created_at", Keys: map[string]int{"created_at": 1}},
		{Name: "idx_email_unique", Keys: map[string]int{"email": 1}, Unique: true},
		{Name: "idx_country_lastname", Keys: map[string]int{"address.country": 1, "last_name": 1}},
	}
}

// Generate implements Template.
func (UserProfile) Generate(r *rand.Rand, fk *gofakeit.Faker, loadRunID string) (bson.D, int, error) {
	doc := bson.D{
		{Key: "_id", Value: MakeID(IDKindObjectID)},
		{Key: "email", Value: fk.Email()},
		{Key: "first_name", Value: fk.FirstName()},
		{Key: "last_name", Value: fk.LastName()},
		{Key: "phone", Value: fk.Phone()},
		{Key: "address", Value: bson.M{
			"street":  fk.Street(),
			"city":    fk.City(),
			"state":   fk.State(),
			"zip":     fk.Zip(),
			"country": fk.Country(),
		}},
		{Key: "preferences", Value: bson.M{
			"language":      fk.Language(),
			"timezone":      fk.TimeZoneRegion(),
			"newsletter":    fk.Bool(),
			"notifications": fk.Bool(),
		}},
		{Key: "bio", Value: pickWords(fk, 24+r.Intn(40))},
	}
	doc = append(doc, defaultCommonFields(loadRunID)...)
	return doc, marshalSize(doc), nil
}

// EventLog models a web/backend event (~500B-1KB).
type EventLog struct{}

// Name implements Template.
func (EventLog) Name() string { return "event_log" }

// IDKind implements Template.
func (EventLog) IDKind() IDKind { return IDKindObjectID }

// IndexSpecs implements Template.
func (EventLog) IndexSpecs() []IndexSpec {
	return []IndexSpec{
		{Name: "idx_created_at", Keys: map[string]int{"created_at": 1}},
		{Name: "idx_user_event", Keys: map[string]int{"user_id": 1, "event": 1, "created_at": -1}},
	}
}

// Generate implements Template.
func (EventLog) Generate(r *rand.Rand, fk *gofakeit.Faker, loadRunID string) (bson.D, int, error) {
	event := fk.RandomString([]string{"page_view", "click", "signup", "login", "logout", "purchase", "search"})
	doc := bson.D{
		{Key: "_id", Value: MakeID(IDKindObjectID)},
		{Key: "user_id", Value: fk.UUID()},
		{Key: "session_id", Value: fk.UUID()},
		{Key: "event", Value: event},
		{Key: "path", Value: fk.URL()},
		{Key: "user_agent", Value: fk.UserAgent()},
		{Key: "ip", Value: fk.IPv4Address()},
		{Key: "metadata", Value: bson.M{
			"referrer": fk.URL(),
			"browser":  fk.RandomString([]string{"Chrome", "Firefox", "Safari", "Edge"}),
			"country":  fk.Country(),
		}},
	}
	doc = append(doc, defaultCommonFields(loadRunID)...)
	return doc, marshalSize(doc), nil
}

// Order models an ecommerce order (~5-10KB).
type Order struct{}

// Name implements Template.
func (Order) Name() string { return "order" }

// IDKind implements Template.
func (Order) IDKind() IDKind { return IDKindObjectID }

// IndexSpecs implements Template.
func (Order) IndexSpecs() []IndexSpec {
	return []IndexSpec{
		{Name: "idx_created_at", Keys: map[string]int{"created_at": 1}},
		{Name: "idx_customer_status", Keys: map[string]int{"customer.email": 1, "status": 1}},
	}
}

// Generate implements Template.
func (Order) Generate(r *rand.Rand, fk *gofakeit.Faker, loadRunID string) (bson.D, int, error) {
	items := make([]bson.M, 1+r.Intn(12))
	var subtotal float64
	for i := range items {
		price := fk.Price(1, 500)
		qty := 1 + r.Intn(4)
		subtotal += price * float64(qty)
		items[i] = bson.M{
			"sku":      fk.LetterN(8),
			"name":     fk.ProductName(),
			"price":    price,
			"quantity": qty,
			"category": fk.ProductCategory(),
		}
	}
	doc := bson.D{
		{Key: "_id", Value: MakeID(IDKindObjectID)},
		{Key: "order_number", Value: fk.Numerify("##########")},
		{Key: "status", Value: fk.RandomString([]string{"pending", "paid", "shipped", "delivered", "refunded"})},
		{Key: "subtotal", Value: subtotal},
		{Key: "tax", Value: subtotal * 0.08},
		{Key: "total", Value: subtotal * 1.08},
		{Key: "customer", Value: bson.M{
			"email":   fk.Email(),
			"name":    fk.Name(),
			"address": fk.Address(),
		}},
		{Key: "items", Value: items},
		{Key: "payment", Value: bson.M{
			"method": fk.RandomString([]string{"card", "paypal", "apple_pay"}),
			"last4":  fk.Numerify("####"),
		}},
	}
	doc = append(doc, defaultCommonFields(loadRunID)...)
	return doc, marshalSize(doc), nil
}

// Telemetry is a small sensor-reading doc (~300-700B).
type Telemetry struct{}

// Name implements Template.
func (Telemetry) Name() string { return "telemetry" }

// IDKind implements Template.
func (Telemetry) IDKind() IDKind { return IDKindObjectID }

// IndexSpecs implements Template.
func (Telemetry) IndexSpecs() []IndexSpec {
	return []IndexSpec{
		{Name: "idx_created_at", Keys: map[string]int{"created_at": 1}},
		{Name: "idx_device_time", Keys: map[string]int{"device_id": 1, "created_at": -1}},
	}
}

// Generate implements Template.
func (Telemetry) Generate(r *rand.Rand, fk *gofakeit.Faker, loadRunID string) (bson.D, int, error) {
	lat, _ := fk.LatitudeInRange(-85, 85)
	lon, _ := fk.LongitudeInRange(-179, 179)
	doc := bson.D{
		{Key: "_id", Value: MakeID(IDKindObjectID)},
		{Key: "device_id", Value: fk.UUID()},
		{Key: "temp_c", Value: 15 + r.Float64()*20},
		{Key: "humidity", Value: r.Float64() * 100},
		{Key: "battery", Value: r.Float64()},
		{Key: "gps", Value: bson.M{"lat": lat, "lon": lon}},
	}
	doc = append(doc, defaultCommonFields(loadRunID)...)
	return doc, marshalSize(doc), nil
}

// DocumentBlob carries ~50-200KB of text to exercise large-document paths.
type DocumentBlob struct{}

// Name implements Template.
func (DocumentBlob) Name() string { return "document_blob" }

// IDKind implements Template.
// UUID _ids spread writes across the B-tree; blobs are rewritten often so
// uniform distribution here produces more realistic fragmentation than the
// default ObjectID right-edge concentration.
func (DocumentBlob) IDKind() IDKind { return IDKindUUID }

// IndexSpecs implements Template.
func (DocumentBlob) IndexSpecs() []IndexSpec {
	return []IndexSpec{
		{Name: "idx_created_at", Keys: map[string]int{"created_at": 1}},
		{Name: "idx_tags", Keys: map[string]int{"tags": 1}},
	}
}

// Generate implements Template.
func (DocumentBlob) Generate(r *rand.Rand, fk *gofakeit.Faker, loadRunID string) (bson.D, int, error) {
	sizeKB := 50 + r.Intn(150)
	content := sizedText(r, sizeKB*1024)
	tags := make([]string, 3+r.Intn(6))
	for i := range tags {
		tags[i] = fk.Word()
	}
	doc := bson.D{
		{Key: "_id", Value: MakeID(IDKindUUID)},
		{Key: "title", Value: fk.Sentence(5)},
		{Key: "author", Value: fk.Name()},
		{Key: "tags", Value: tags},
		{Key: "content", Value: content},
	}
	doc = append(doc, defaultCommonFields(loadRunID)...)
	return doc, marshalSize(doc), nil
}

// IoTTimeseries is a batched reading vector (~1-3KB).
type IoTTimeseries struct{}

// Name implements Template.
func (IoTTimeseries) Name() string { return "iot_timeseries" }

// IDKind implements Template.
func (IoTTimeseries) IDKind() IDKind { return IDKindObjectID }

// IndexSpecs implements Template.
func (IoTTimeseries) IndexSpecs() []IndexSpec {
	return []IndexSpec{
		{Name: "idx_created_at", Keys: map[string]int{"created_at": 1}},
		{Name: "idx_sensor_time", Keys: map[string]int{"sensor_id": 1, "created_at": -1}},
	}
}

// Generate implements Template.
func (IoTTimeseries) Generate(r *rand.Rand, fk *gofakeit.Faker, loadRunID string) (bson.D, int, error) {
	readings := make([]bson.M, 20+r.Intn(30))
	for i := range readings {
		readings[i] = bson.M{
			"t": r.Int63n(3_600_000),
			"v": r.Float64() * 100,
		}
	}
	doc := bson.D{
		{Key: "_id", Value: MakeID(IDKindObjectID)},
		{Key: "sensor_id", Value: fk.UUID()},
		{Key: "kind", Value: fk.RandomString([]string{"flow", "pressure", "temperature", "vibration"})},
		{Key: "readings", Value: readings},
		{Key: "firmware", Value: fk.AppVersion()},
	}
	doc = append(doc, defaultCommonFields(loadRunID)...)
	return doc, marshalSize(doc), nil
}
