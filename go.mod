module simulation

go 1.20

require (
	github.com/davecgh/go-spew v0.0.0
	github.com/joho/godotenv v0.0.0
	github.com/tealeg/xlsx v0.0.0
)

replace github.com/davecgh/go-spew => ./vendor/github.com/davecgh/go-spew

replace github.com/joho/godotenv => ./vendor/github.com/joho/godotenv

replace github.com/tealeg/xlsx => ./vendor/github.com/tealeg/xlsx
