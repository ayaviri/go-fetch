# go-fetch
receipt procesor challenge for fetch

## running local server

```
$ chmod +x dev.sh
$ ./dev.sh
```

## notes for the evaluator
the web server makes use of the components provided by the `http` package of the standard library and a map from receipt IDs to Receipt structs, protected by a mutex to ensure safe access by concurrent goroutines (just in case). i hope my decision to keep everything in one file doesn't make it too difficult to parse through
