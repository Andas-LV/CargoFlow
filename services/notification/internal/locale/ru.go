// Package locale translates event codes into Russian for notifications.
package locale

var cargoTypes = map[string]string{
	"wagon":     "вагон",
	"container": "контейнер",
}

var statuses = map[string]string{
	"created":    "Создана",
	"in_transit": "В пути",
	"delivered":  "Доставлена",
	"cancelled":  "Отменена",
}

// CargoType returns the Russian name of a cargo type, or the code itself if unknown.
func CargoType(code string) string { return translate(cargoTypes, code) }

// Status returns the Russian name of a shipment status, or the code itself if unknown.
func Status(code string) string { return translate(statuses, code) }

func translate(dict map[string]string, code string) string {
	if v, ok := dict[code]; ok {
		return v
	}
	return code
}
