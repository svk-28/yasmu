package main

import (
    "net/http"
    "html/template"
)

func Start() {
    // API маршруты
    http.HandleFunc("/api/user", api.GetUserHandler)
    
    // Веб-интерфейс
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        tmpl := template.Must(template.ParseFiles("web/templates/index.html"))
        tmpl.Execute(w, nil)
    })                
    
    // Статика
    http.Handle("/static/", http.StripPrefix("/static/", 
        http.FileServer(http.Dir("web/static"))))
    
    http.ListenAndServe(":8080", nil)
}    
