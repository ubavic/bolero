# Static properties

Static properties are constant metadata attached, in Bolero source files, to package-level struct types, struct fields, enum types and enum values. They take the place of struct tags. Bolero code reads them at compile time with `@`. Any code reads them at run time through `bolero/meta`.

Struct tags are not allowed in Bolero files.

## Grammar

A property block is written after the declaration it describes.

```
PropertyBlock = "{" [ PropertyList [ "," ] ] "}" .
PropertyList  = Property { "," Property } .
Property      = identifier [ ":" Expression ] .
```

A property without a value is a flag, equivalent to `Name: true`. Names are separated by commas, as in a composite literal. A trailing comma is optional when `}` is on the same line, and required before a newline, because of automatic semicolon insertion.

```
FieldDecl = identifier Type [ PropertyBlock ] .
TypeDecl  = "type" identifier Type [ PropertyBlock ] .
EnumValue = identifier [ PropertyBlock ] .
```

The block attaches to:

- a struct field, after the field's type;
- a struct or enum type, after the type in a type declaration;
- an enum value, after the value's name.

A name may appear only once in a block. Names are not drawn from a fixed set: a misspelled name is reported where the property is read, not where it is declared.

The value must be a constant: a numeric, string or boolean constant, a named constant, a constant expression, or an enum value. The blank identifier is not a property name.

`@` is a postfix operator with the precedence of a selector. It is recognized only in Bolero files.

```
PrimaryExpr = PrimaryExpr "@" identifier .
```

## Where properties are allowed

Properties are allowed only on struct and enum types declared at package level.

- A generic type cannot have properties.
- A defined type that is neither a struct nor an enum cannot have properties: `type Money int { Unit: "usd" }` is an error.
- A property block on a field is allowed only on the struct type in a type declaration, not on an anonymous struct.
- A property block applies to one field name. `A, B int { MaxLength: 5 }` is an error.
- An embedded field cannot have properties. A promoted field can still be named with `::`; its properties are those of the field it comes from.

## Reading properties with `@`

`X@Name` is the constant value of the property `Name` of the declaration that the static path `X` denotes. `X` is one of:

- a type name, possibly qualified (`Person`, `pkg.Person`);
- a field path (`Person::Name`);
- an enum value (`Status::Archived`).

`@` does not apply to variables or other values. In particular, `f@MaxLength` is an error when `f` has type `meta.Field[...]`; call `f.Properties()` instead.

The result is a constant. An untyped constant stays untyped, so `Person::Name@MaxLength` can be used as an `int` or a `uint`. A typed constant, such as an enum value, keeps its type.

Reading a property that was not declared is an error at the read:

```
Person::Name has no property MaxLenght
```

Reading with `@` does not require `import "bolero/meta"`.

## Field descriptors

For a struct type `T` with a field `F` of type `FT`, `T::F` is a value of type `meta.Field[T, FT]`. A file that uses `T::F` as a value must import `"bolero/meta"`.

`Color::Red` is unchanged: it is an enum value, not a descriptor.

Promoted fields work. If `Employee` embeds `Person`, then `Employee::Name` is a descriptor of `Employee` for the promoted field, and its properties are those of `Person.Name`. Unexported fields follow Go's visibility rules. Descriptors are comparable: `f == Person::Name`.

`meta.Field` provides:

```
func (f Field[S, T]) Name() string
func (f Field[S, T]) Get(s S) T
func (f Field[S, T]) Set(s *S, v T)
func (f Field[S, T]) Properties() Properties
```

`Get` takes the struct by value. `Properties` returns the field's static properties. `Names` on `Properties` returns the names in declaration order, and `Lookup` returns the value.

## Run-time metadata

```
func TypeProperties[T any]() Properties
func ValueProperties[E ~int](v E) Properties
```

`TypeProperties[T]()` returns the properties of the struct or enum type `T`. `ValueProperties(v)` returns the properties of the enum value `v`. Both return the zero `Properties` when nothing was declared.

At run time, an untyped constant is stored as its default type (`int`, `float64`, `complex128`, `string`, `bool`; an untyped rune becomes `int32`). A typed constant keeps its type. `Lookup` returns that value as an `any`.

Every package that contains a Bolero file depends on `bolero/meta`, in the same way that every Go package depends on `runtime`. The compiler generates an `init` in the declaring package that registers the type and enum-value properties. Source code still imports `bolero/meta` explicitly wherever it writes `meta.` or uses a descriptor value. `bolero/meta` itself is written in Go and contains no Bolero files.

`TypeProperties` and `ValueProperties` are valid once the declaring package has been initialized. An imported package is initialized first, so a variable initializer in an importing package can call them. A variable initializer in the declaring package itself runs before that package's `init` functions.

## String and Label

The generated `String` method of a package-level enum returns a value's `Label` property when that property is a string, and the value's name otherwise. A `Label` of some other type is an error. A `String` method declared in the package replaces the generated method entirely, as for enums without properties.

## Lowering to Go

A property block becomes data passed to `meta.NewProperties`. The declaring package's generated `init` calls `meta.RegisterType` and `meta.RegisterValue`. A descriptor `Person::Name` becomes a call of `meta.NewField` with the field name, its properties, and functions that get and set the field. `NewField` returns the same descriptor for every call with the same struct type and field name, which is what makes descriptors comparable.

`NewProperties`, `NewField`, `RegisterType` and `RegisterValue` are part of `bolero/meta` so that the lowering has a plain-Go equivalent. Programs normally use `@`, `::`, `TypeProperties` and `ValueProperties` instead of calling them directly.

## Examples

```
import "bolero/meta"

type Person struct {
    Name  string { Required, MaxLength: 50, Label: "Full name" }
    Email string { MaxLength: 100, Label: "E-mail" }
    Age   uint
} { Table: "people" }

type Status enum {
    Active   { Label: "Active" }
    Archived { Label: "Archived", Hidden: true }
} { Label: "Status" }

const (
    table  = Person@Table           // "people"
    maxLen = Person::Name@MaxLength // 50
    hidden = Status::Archived@Hidden
)

func rename(p *Person, name string) {
    if len(name) > Person::Name@MaxLength {
        return
    }
    var f meta.Field[Person, string] = Person::Name
    f.Set(p, name)
}
```
