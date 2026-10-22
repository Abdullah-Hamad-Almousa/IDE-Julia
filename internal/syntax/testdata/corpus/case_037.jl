function guarded037(f)
    try
        return f()
    catch err
        showerror(stderr, err)
    finally
        println("done")
    end
end
