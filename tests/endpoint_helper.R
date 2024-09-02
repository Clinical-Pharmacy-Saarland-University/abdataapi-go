rmTrailingSlash <- function(input_string) {
  return(sub("/$", "", input_string))
}

# parse data string from API to R datetime in local timezone
parseDate <- function(date_str) {
  lubridate::as_datetime(date_str, tz = Sys.timezone()) |>
    suppressMessages()
}

# post just a body without any authentication
post <- function(url, body) {
  res <- httr2::request(url) |>
    httr2::req_body_json(body) |>
    httr2::req_error(body = .error_body) |>
    httr2::req_perform() |>
    httr2::resp_body_json()
  return(res)
}

getReq <- function(url) {
  res <- httr2::request(url) |>
    httr2::req_error(body = .error_body) |>
    httr2::req_perform() |>
    httr2::resp_body_json()
  return(res)
}

getR6Auth <- function(login, endpoint, query, auto_refresh = TRUE, verbose = FALSE) {
  .authQuery("GET", login, endpoint, query, auto_refresh, verbose)
}

postR6Auth <- function(login, endpoint, body, auto_refresh = TRUE, verbose = FALSE) {
  .authQuery("POST", login, endpoint, body, auto_refresh, verbose)
}

patchR6Auth <- function(login, endpoint, body, auto_refresh = TRUE, verbose = FALSE) {
  .authQuery("PATCH", login, endpoint, body, auto_refresh, verbose)
}

deleteR6Auth <- function(login, endpoint, body, auto_refresh = TRUE, verbose = FALSE) {
  .authQuery("DELETE", login, endpoint, body, auto_refresh, verbose)
}

.error_body_format <- function(body) {
  if (!is.null(body[["error"]])) {
    return(body$error)
  }

  if (!is.null(body[["errors"]])) {
    errs <- body$errors
    res <- purrr::map_vec(errs, \(x) glue::glue("Query/field '{x$field}': {x$reason}"))
    return(res)
  }

  if (!is.null(body[["message"]])) {
    return(body$message)
  }

  return("Unparsable error message.")
}


.error_body <- function(resp) {
  if (httr2::resp_has_body(resp)) {
    if (httr2::resp_content_type(resp) == "application/json") {
      err_body <- httr2::resp_body_json(resp)
      return(.error_body_format(err_body))
    } else {
      return(httr2::resp_body_string(resp))
    }
  } else {
    return(NULL)
  }
}


.authQuery <- function(
    method, login, endpoint, body_or_query,
    auto_refresh = TRUE, verbose = FALSE) {
  if (auto_refresh && !login$accessValid()) {
    login$refresh()
  }

  url <- paste0(login$baseUrl(), endpoint)
  token <- login$accessToken()

  req <- httr2::request(url) |>
    httr2::req_auth_bearer_token(token) |>
    httr2::req_error(body = .error_body) |>
    httr2::req_method(method)

  if (method == "GET" && !is.null(body_or_query)) {
    req <- req |> httr2::req_url_query(!!!body_or_query)
  } else {
    req <- req |> httr2::req_body_json(body_or_query)
  }

  if (verbose) {
    req <- req |> httr2::req_verbose(body_req = TRUE, body_resp = TRUE)
  }

  res <- req |>
    httr2::req_perform() |>
    httr2::resp_body_json()

  return(res)
}


listToDf <- function(list) {
  list <- purrr::map(list, ~ purrr::map(.x, ~ ifelse(is.null(.x), NA, .x)))
  dplyr::bind_rows(list)
}
