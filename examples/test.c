#include <SDL3/SDL.h>
#include <stdio.h>

int main() {
    SDL_Init(SDL_INIT_VIDEO);
    SDL_Window *window = SDL_CreateWindow("Test", 640, 480, 0);
    printf("window = %p\n", window);
    
    SDL_Renderer *renderer = SDL_CreateRenderer(window, NULL);
    printf("renderer = %p\n", renderer);
    
    if (!renderer) {
        printf("Error: %s\n", SDL_GetError());
    } else {
        printf("Success!\n");
    }
    
    SDL_Quit();
    return 0;
}
