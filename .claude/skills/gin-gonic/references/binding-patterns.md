# Gin Binding Patterns Reference

Complete guide to request binding and validation in Gin.

## Binding Overview

Gin supports binding from multiple sources:
- JSON body
- XML body
- Form data (urlencoded or multipart)
- Query string
- URI path parameters
- Headers

## Struct Tags

### Core Tags

| Tag | Purpose | Example |
|-----|---------|---------|
| `json` | JSON field name | `json:"user_name"` |
| `form` | Form/query field name | `form:"user_name"` |
| `uri` | URI parameter name | `uri:"id"` |
| `header` | Header name | `header:"X-Request-ID"` |
| `xml` | XML element name | `xml:"user_name"` |
| `binding` | Validation rules | `binding:"required,min=3"` |

### Common Validation Rules

```go
type User struct {
    // Required field
    Name string `binding:"required"`
    
    // String length
    Username string `binding:"required,min=3,max=20"`
    
    // Numeric range
    Age int `binding:"gte=0,lte=130"`
    
    // Email validation
    Email string `binding:"required,email"`
    
    // URL validation
    Website string `binding:"url"`
    
    // UUID validation
    ID string `binding:"uuid"`
    
    // One of allowed values
    Status string `binding:"oneof=active inactive pending"`
    
    // Field comparison
    Password        string `binding:"required,min=8"`
    ConfirmPassword string `binding:"required,eqfield=Password"`
    
    // Conditional required
    Phone string `binding:"required_without=Email"`
    
    // Skip validation
    Internal string `binding:"-"`
}
```

### Time Formats

```go
type Event struct {
    // Standard date format
    Date time.Time `form:"date" time_format:"2006-01-02"`
    
    // With timezone
    DateTime time.Time `form:"datetime" time_format:"2006-01-02T15:04:05Z07:00" time_utc:"1"`
    
    // Unix timestamp
    UnixTime time.Time `form:"unix" time_format:"unix"`
    
    // Unix milliseconds
    UnixMilli time.Time `form:"unix_milli" time_format:"unixmilli"`
    
    // Unix nanoseconds
    UnixNano time.Time `form:"unix_nano" time_format:"unixNano"`
}
```

### Default Values

```go
type Pagination struct {
    Page     int    `form:"page,default=1"`
    PageSize int    `form:"page_size,default=20"`
    Sort     string `form:"sort,default=created_at"`
}
```

### Collection Formats

```go
type Filter struct {
    // Default: multi (key=a&key=b)
    Tags []string `form:"tags"`
    
    // CSV: tags=a,b,c
    Categories []string `form:"categories" collection_format:"csv"`
    
    // SSV (space-separated): items=a b c
    Items []string `form:"items" collection_format:"ssv"`
    
    // Pipes: values=a|b|c
    Values []string `form:"values" collection_format:"pipes"`
}
```

## Binding Methods Comparison

### Must Bind vs Should Bind

```go
// Must Bind - Aborts with 400 on error (less flexible)
func handleMustBind(c *gin.Context) {
    var req Request
    c.Bind(&req)  // Automatically aborts on error
    // Code here only runs if binding succeeded
}

// Should Bind - Returns error for custom handling (recommended)
func handleShouldBind(c *gin.Context) {
    var req Request
    if err := c.ShouldBind(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error":   "validation_failed",
            "details": err.Error(),
        })
        return
    }
    // Process valid request
}
```

### Binding from Different Sources

```go
// Auto-detect based on Content-Type
c.ShouldBind(&obj)

// Explicit JSON
c.ShouldBindJSON(&obj)

// Query string only
c.ShouldBindQuery(&obj)

// URI parameters only
c.ShouldBindUri(&obj)

// Headers only
c.ShouldBindHeader(&obj)

// Form data (POST body or query)
c.ShouldBind(&obj)  // Auto-detects

// Multiple sources combined
func handler(c *gin.Context) {
    var uri UriParams
    var query QueryParams
    var body BodyParams
    
    if err := c.ShouldBindUri(&uri); err != nil { ... }
    if err := c.ShouldBindQuery(&query); err != nil { ... }
    if err := c.ShouldBindJSON(&body); err != nil { ... }
}
```

## Multiple Binds from Body

Body can only be read once. Use `ShouldBindBodyWith` for multiple binds:

```go
func handler(c *gin.Context) {
    var formA FormA
    var formB FormB
    
    // First bind stores body in context
    if err := c.ShouldBindBodyWith(&formA, binding.JSON); err != nil {
        // handle formA
    }
    
    // Second bind reuses stored body
    if err := c.ShouldBindBodyWith(&formB, binding.JSON); err != nil {
        // handle formB
    }
}

// Shortcuts available:
c.ShouldBindBodyWithJSON(&obj)
c.ShouldBindBodyWithXML(&obj)
c.ShouldBindBodyWithYAML(&obj)
```

## Nested Structs

```go
type Address struct {
    Street string `form:"street" json:"street"`
    City   string `form:"city" json:"city"`
}

type Person struct {
    Name    string  `form:"name" json:"name"`
    Address Address // Nested struct
}

// Binds from: name=John&street=Main&city=NYC
// Or JSON: {"name": "John", "address": {"street": "Main", "city": "NYC"}}
```

## Custom Validators

```go
import (
    "github.com/gin-gonic/gin/binding"
    "github.com/go-playground/validator/v10"
)

// Custom validation function
var bookableDate validator.Func = func(fl validator.FieldLevel) bool {
    date, ok := fl.Field().Interface().(time.Time)
    if ok {
        return !time.Now().After(date)
    }
    return true
}

func main() {
    r := gin.Default()
    
    // Register custom validator
    if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
        v.RegisterValidation("bookabledate", bookableDate)
    }
    
    r.GET("/book", func(c *gin.Context) {
        var b Booking
        if err := c.ShouldBind(&b); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
            return
        }
        c.JSON(http.StatusOK, gin.H{"message": "valid"})
    })
}

type Booking struct {
    CheckIn  time.Time `form:"check_in" binding:"required,bookabledate" time_format:"2006-01-02"`
    CheckOut time.Time `form:"check_out" binding:"required,gtfield=CheckIn" time_format:"2006-01-02"`
}
```

## Custom Unmarshaler

```go
type Birthday string

func (b *Birthday) UnmarshalParam(param string) error {
    *b = Birthday(strings.Replace(param, "-", "/", -1))
    return nil
}

// Now Birthday will automatically transform "2000-01-01" to "2000/01/01"
```

## File Upload Binding

```go
type UploadForm struct {
    Name   string                `form:"name" binding:"required"`
    Avatar *multipart.FileHeader `form:"avatar" binding:"required"`
    
    // Multiple files
    Photos []*multipart.FileHeader `form:"photos" binding:"required"`
}

func uploadHandler(c *gin.Context) {
    var form UploadForm
    if err := c.ShouldBind(&form); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Save single file
    c.SaveUploadedFile(form.Avatar, "./uploads/avatar.jpg")
    
    // Save multiple files
    for i, photo := range form.Photos {
        c.SaveUploadedFile(photo, fmt.Sprintf("./uploads/photo_%d.jpg", i))
    }
}
```

## HTML Checkbox Binding

```go
type FormData struct {
    Colors []string `form:"colors[]"`
}

// HTML:
// <input type="checkbox" name="colors[]" value="red">
// <input type="checkbox" name="colors[]" value="blue">

func handler(c *gin.Context) {
    var form FormData
    c.ShouldBind(&form)
    // form.Colors = ["red", "blue"] if both checked
}
```

## Map Binding

```go
// Query: ids[a]=1&ids[b]=2
// Body: names[first]=John&names[second]=Jane

func handler(c *gin.Context) {
    ids := c.QueryMap("ids")       // map[string]string{"a": "1", "b": "2"}
    names := c.PostFormMap("names") // map[string]string{"first": "John", "second": "Jane"}
}
```

## Error Handling Best Practices

```go
func handler(c *gin.Context) {
    var req CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        // Type assert to get validation errors
        var ve validator.ValidationErrors
        if errors.As(err, &ve) {
            out := make([]ApiFieldError, len(ve))
            for i, fe := range ve {
                out[i] = ApiFieldError{
                    Field:   fe.Field(),
                    Message: getErrorMsg(fe),
                }
            }
            c.JSON(http.StatusBadRequest, gin.H{"errors": out})
            return
        }
        
        // Other binding errors (JSON parse, etc.)
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }
    
    // Process valid request...
}

type ApiFieldError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

func getErrorMsg(fe validator.FieldError) string {
    switch fe.Tag() {
    case "required":
        return "This field is required"
    case "email":
        return "Invalid email format"
    case "min":
        return fmt.Sprintf("Minimum length is %s", fe.Param())
    default:
        return "Invalid value"
    }
}
```