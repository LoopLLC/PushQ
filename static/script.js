/**
 * JS Utility Functions.
 * 
 * e.g.
 * var pushq = new Pushq('/api');
 */
var Pushq = function(apiBaseUrl) { 
    if (!apiBaseUrl) apiBaseUrl = "/admin";
    if (!(this instanceof arguments.callee)) {
        return new Pushq(apiBaseUrl); // var pushq = Pushq('/api');
    }
    this.apiBaseUrl = apiBaseUrl;
}

/**
 * Get an element by id.
 */
Pushq.prototype.id = function(id) {
    return document.getElementById(id);
};

/**
 * Get the first element in the div with the class name.
 */
Pushq.prototype.cls = function(div, className) {
    var els = div.getElementsByClassName(className);
    if (!els || els.length == 0) {
        return null;
    }
    return els[0];
};

/**
 * Bind an anchor click to a function in this class.
 */
Pushq.prototype.bindClick = function(id, f) {
    this.id(id).addEventListener('click', f);
};


// Get the selected value from a select element
Pushq.prototype.getSelected = function(selectId) {
    var sel = this.id(selectId);
    return sel.options[sel.selectedIndex].value;
};

/**
 * Add an option to a select
 */
Pushq.prototype.addOption = function(sel, val, txt) {
    var o = document.createElement('option');
    o.value = val;
    o.innerHTML = txt;
    sel.appendChild(o);
};

/**
 * Make a GET request and pass the data to the callback function
 */
Pushq.prototype.getData = function(uri, success, fail) {
    var xhr = new XMLHttpRequest();
    xhr.open('GET', encodeURI(uri));
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.setRequestHeader('Accept', 'application/json');
    xhr.onload = function () {
        if (xhr.status === 200) {
            var t = this.responseText;
            var data = {};
            if (t === undefined || t == 'undefined' || t == '') {
                console.log('getData undefined response text');
            } else {
                data = JSON.parse(t);
            }
            success(data);
        }
        else {
            console.log('getData failed: ' + xhr.status);
            fail(xhr.status);
        }
    };
    xhr.send();
};

/**
 * Get data from the API and pass it to the callback function.
 */
Pushq.prototype.getApiData = function(apiMethod, success, fail) {
    this.getData(this.apiBaseUrl + '/' + apiMethod, success, fail);
};

/**
 * Make a GET request and call callback(content)
 */
Pushq.prototype.getHtml = function(uri, callback) {
    var xhr = new XMLHttpRequest();
    xhr.open('GET', encodeURI(uri));
    xhr.onload = function() {
        if (xhr.status === 200 && xhr.response.d != 'Failed') { 
            callback(xhr.responseText);
        }
        else { 
            console.log('Request failed: ' + xhr.status);
        }
    };
    xhr.send();
};

/**
 * Get content and insert it into the element.
 */
Pushq.prototype.insertContent = function (uri, el) {
    this.getHtml(uri, function (content) {
        el.innerHTML = content;
    });
};

/**
 * Converts data to JSON and POSTs
 * The success and fail functions accept the response returned from the API
 */
Pushq.prototype.post = function(uri, data, success, fail) {
    var xhr = new XMLHttpRequest();
    xhr.open('POST', uri);
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.setRequestHeader('Accept', 'application/json');
    xhr.onload = function () {
        // Response object from API must have boolean Ok
        if (this.status === 200) {
            var t = this.responseText;
            var r = {};
            if (t === undefined || t == 'undefined' || t == '') {
                console.log('Undefined response text, assuming Ok');
                r.ok = true; // assume Ok if blank response
            } else {
                r = JSON.parse(t);
            }
            if (!r.ok) {
                if (fail) fail(r);
            } else {
                if (success) success(r);
            }
        } else {
            console.log('post got status ' + this.status);
            var r = {};
            r.ok = false;
            if (fail) {
                fail(r);
            }
        }
    };
    xhr.send(JSON.stringify(data));
};

/**
 * Post data to an API method.
 */
Pushq.prototype.postApi = function(apiMethod, data, success, fail) {
    this.post(this.apiBaseUrl + '/' + apiMethod, data, success, fail); 
};

/**
 * Find the first element in div with className and append the html in a new div.
 * Returns the newly created div.
 */
Pushq.prototype.appendHtml = function (div, className, html) {
    var els = div.getElementsByClassName(className);
    if (!els || els.length == 0) return null;
    var div = document.createElement('div');
    div.innerHTML = html;
    els[0].appendChild(div);
    return div;
};

/**
 * Get the object's property names.
 */
Pushq.prototype.getPropertyNames = function (object) {
    var names = [];
    for (var propName in object) { 
        if (object.hasOwnProperty(propName)) { 
            names.push(propName);
        }
    }
    return names;
};

/**
 * Find the first element with className in div and set innerText to text.
 */
Pushq.prototype.clsText = function (div, className, text) {
    var el = this.cls(div, className);
    if (el) {
        el.innerText = text;
    }
}

/**
 * Display an alert message.
 */
Pushq.prototype.alert = function(msg, level) {
    var el = this.id("popup");
    el.className = "popupShow";
    var bg = "lightgreen";
    switch (level) {
        case "warn":
            bg = "lightyellow";
            break;
        case "error":
            bg = "pink";
            break;
        default: break;
    }
    el.style.backgroundColor = bg;
    this.id("alertMsg").innerText = msg;
}

/**
 * Hide the alert message.
 */
Pushq.prototype.closeAlert = function() {
    var el = this.id("popup");
    el.className = "popupHide";
    this.id("alertMsg").innerText = "";
}

/**
 * Send the selected value to the server and put the returned content
 * into the specified div.
 */
Pushq.prototype.selectContent = function(selectId, uri, divId) {
    var pushq = window.pushq;
    var selectedValue = pushq.getSelected(selectId);
    var uri = "/" + uri + "/"  + encodeURIComponent(selectedValue);
    pushq.insertContent(uri, pushq.id(divId));
}

/**
 * Create a new API Key.
 */
Pushq.prototype.newKey = function() {
    var pushq = this;
    pushq.getApiData("newapikey", 
    function(r) {
        var data = r.data;
        pushq.alert("This is the last time you will see the Secret, so be sure to store it securely now", 
        "alert");
        var el = pushq.id("showkey");
        el.innerText = "Key: " + 
            data.Key + ", Secret: " + data.Secret;
    }, 
    function(msg) {
        pushq.alert(msg, "error");
    });
}

/**
 * Delete an API Key.
 */
Pushq.prototype.deleteKey = function(key) {
    var pushq = this;
    pushq.postApi("delapikey", { Key: key }, 
    function() {
        window.location = "/admin/keys";
        //pushq.alert("Deleted")
    }, function(msg) {
        pushq.alert(msg, "error");
    })
}