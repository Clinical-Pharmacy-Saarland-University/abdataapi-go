source("./tests/endpoint_helper.R")


tictoc::tic()
response <- httr::GET("127.0.0.1:3333/api/formulations")
res <- httr::content(response, "text", encoding = "UTF-8") |>
  jsonlite::fromJSON()
tictoc::toc()


tictoc::tic()
response <- httr::GET("127.0.0.1:3333/api/interactions/pzns?pzns=03041347,05538454,13880764,00189747,01970060,00054065,17145955,00592733,13981502")
tictoc::toc()
res <- httr::content(response, "text", encoding = "UTF-8") |>
  jsonlite::fromJSON()
res


pzn_list <- purrr::map(seq(100), \(i)  {
  list(id = jsonlite::unbox(as.character(i)), pzns = c("03041347", "05538454", "13880764", "00189747", "01970060", "00054065", "17145955", "00592733", "13981502"))
})

tictoc::tic()
response <- httr::POST("127.0.0.1:3333/api/interactions/pzns", body = pzn_list, encode = "json")
tictoc::toc()
res <- httr::content(response, "text", encoding = "UTF-8") |>
  jsonlite::fromJSON()
res
